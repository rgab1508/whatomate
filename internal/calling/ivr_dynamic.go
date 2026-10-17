package calling

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/flowgraph"
	"github.com/shridarpatil/whatomate/internal/models"
)

// Synthesizer turns text into an audio file in the calling audio directory and
// returns its filename. Implemented by tts.PiperTTS; same text maps to the same
// file, so repeated prompts are served from cache.
type Synthesizer interface {
	Generate(text string) (string, error)
}

// SetTTS enables prompts whose text is only known during the call (text with
// {{variables}}). Without it such prompts fall back to their audio_file.
func (m *Manager) SetTTS(s Synthesizer) {
	m.tts = s
}

// maxDynamicMenuOptions caps a dynamic menu to the single-key digits 1-9.
const maxDynamicMenuOptions = 9

// hasTemplateVars reports whether text contains a {{variable}} placeholder.
// Such text can't be synthesized when the flow is saved; it is spoken live.
func hasTemplateVars(text string) bool {
	return strings.Contains(text, "{{")
}

// promptAudioPath resolves the audio a greeting/menu/gather/hangup node plays.
// Text containing {{variables}} is filled in from the call's variables and
// synthesized now; anything else uses the audio_file prepared at save time.
// Returns "" when there is nothing to play.
func (m *Manager) promptAudioPath(session *CallSession, node *IVRNode, ctx *IVRContext) string {
	if m.config.AudioDir == "" {
		return ""
	}

	text, _ := node.Config["greeting_text"].(string)
	if hasTemplateVars(text) {
		if m.tts == nil {
			m.log.Error("IVR prompt has variables but TTS is not configured", "call_id", session.ID, "node_id", node.ID)
		} else {
			spoken := strings.TrimSpace(interpolateTemplate(text, ctx.Variables))
			if spoken == "" {
				return ""
			}
			start := time.Now()
			filename, err := m.tts.Generate(spoken)
			if err != nil {
				m.log.Error("Live TTS failed", "error", err, "call_id", session.ID, "node_id", node.ID)
			} else {
				m.log.Info("Live TTS ready", "call_id", session.ID, "node_id", node.ID, "ms", time.Since(start).Milliseconds())
				return filepath.Join(m.config.AudioDir, filename)
			}
		}
	}

	audioFile, _ := node.Config["audio_file"].(string)
	if audioFile == "" {
		return ""
	}
	return filepath.Join(m.config.AudioDir, audioFile)
}

// applyResponseMapping stores fields from a JSON response body into call
// variables. Paths use the chatbot api_call syntax: "data.slots[0].id".
// Strings are stored as-is; numbers and booleans as their text; objects and
// arrays as JSON, so an array can feed a dynamic menu's options_variable.
// The decoded value is also kept in ctx.Values for condition expressions.
func applyResponseMapping(body string, mapping map[string]any, ctx *IVRContext) (int, error) {
	if len(mapping) == 0 {
		return 0, nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return 0, fmt.Errorf("response is not JSON: %w", err)
	}

	mapped := 0
	for varName, rawPath := range mapping {
		path, _ := rawPath.(string)
		if varName == "" || path == "" {
			continue
		}
		value, ok := lookupJSONPath(parsed, path)
		if !ok {
			continue
		}
		ctx.Variables[varName] = jsonValueString(value)
		if ctx.Values != nil {
			ctx.Values[varName] = value
		}
		mapped++
	}
	return mapped, nil
}

// lookupJSONPath walks a decoded JSON value by a dotted path with optional
// array indexes ("options[1].label", "[0]"). A trailing ".length" on an array
// yields its size.
func lookupJSONPath(data any, path string) (any, bool) {
	current := data
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, false
		}
		if part == "length" {
			if arr, ok := current.([]any); ok {
				current = float64(len(arr))
				continue
			}
		}

		field, indexes, ok := splitIndexes(part)
		if !ok {
			return nil, false
		}
		if field != "" {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			if current, ok = obj[field]; !ok {
				return nil, false
			}
		}
		for _, idx := range indexes {
			arr, ok := current.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			current = arr[idx]
		}
	}
	return current, true
}

// splitIndexes splits "slots[0][1]" into "slots" and [0, 1].
func splitIndexes(part string) (string, []int, bool) {
	open := strings.Index(part, "[")
	if open == -1 {
		return part, nil, true
	}
	field := part[:open]
	var indexes []int
	rest := part[open:]
	for rest != "" {
		if rest[0] != '[' {
			return "", nil, false
		}
		end := strings.Index(rest, "]")
		if end == -1 {
			return "", nil, false
		}
		idx, err := strconv.Atoi(rest[1:end])
		if err != nil {
			return "", nil, false
		}
		indexes = append(indexes, idx)
		rest = rest[end+1:]
	}
	return field, indexes, true
}

// jsonValueString renders a decoded JSON value as a call variable.
func jsonValueString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// dynamicMenuOption is one choice of a menu whose options come from a variable.
type dynamicMenuOption struct {
	Digit string
	Label string
	Value string
}

// parseDynamicOptions reads a menu's options from a JSON array held in a call
// variable. Items may be objects ({"digit","label","value"}; digit optional)
// or plain values. Items without a digit are numbered 1, 2, 3… in order.
// Only single keys 0-9, * and # are usable; the list is capped at nine.
func parseDynamicOptions(raw string) ([]dynamicMenuOption, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("options are not a JSON array: %w", err)
	}

	var opts []dynamicMenuOption
	used := make(map[string]bool)
	next := 1
	for _, item := range items {
		if len(opts) == maxDynamicMenuOptions {
			break
		}
		var opt dynamicMenuOption
		if obj, ok := item.(map[string]any); ok {
			opt.Digit = jsonValueString(obj["digit"])
			opt.Label = jsonValueString(obj["label"])
			opt.Value = jsonValueString(obj["value"])
			if opt.Value == "" {
				opt.Value = opt.Label
			}
		} else {
			opt.Value = jsonValueString(item)
			opt.Label = opt.Value
		}

		if opt.Digit == "" {
			for used[strconv.Itoa(next)] {
				next++
			}
			if next > 9 {
				break
			}
			opt.Digit = strconv.Itoa(next)
			next++
		}
		if len(opt.Digit) != 1 || !strings.ContainsAny(opt.Digit, "0123456789*#") || used[opt.Digit] {
			continue
		}
		used[opt.Digit] = true
		opts = append(opts, opt)
	}
	return opts, nil
}

// executeDynamicMenu runs a menu whose valid keys come from a variable filled
// earlier in the call (usually by an HTTP node's response mapping). On a valid
// key it stores the chosen option and returns "selected"; with no options it
// returns "no_options" without prompting.
func (m *Manager) executeDynamicMenu(session *CallSession, node *IVRNode, ctx *IVRContext, player *AudioPlayer) string {
	varName, _ := node.Config["options_variable"].(string)
	opts, err := parseDynamicOptions(ctx.Variables[varName])
	if err != nil {
		m.log.Error("Dynamic menu options unreadable", "error", err, "call_id", session.ID, "variable", varName)
	}
	if len(opts) == 0 {
		m.log.Info("Dynamic menu has no options", "call_id", session.ID, "node_id", node.ID, "variable", varName)
		return "no_options"
	}

	byDigit := make(map[string]dynamicMenuOption, len(opts))
	for _, o := range opts {
		byDigit[o.Digit] = o
	}

	digit, outcome := m.promptForDigit(session, node, ctx, player, func(d string) bool {
		_, ok := byDigit[d]
		return ok
	})
	if outcome != "" {
		return outcome
	}

	chosen := byDigit[digit]
	ctx.Variables["menu_"+node.ID] = digit
	ctx.Variables["last_menu_digit"] = digit
	ctx.Variables["selected_option_value"] = chosen.Value
	ctx.Variables["selected_option_label"] = chosen.Label
	if storeAs, _ := node.Config["store_as"].(string); storeAs != "" {
		ctx.Variables[storeAs] = chosen.Value
	}
	m.log.Info("Dynamic menu selection", "call_id", session.ID, "digit", digit, "value", chosen.Value)
	return "selected"
}

// executeDelay pauses the flow for config "seconds" (0.5–60). It returns early
// if the call ends meanwhile, so a hung-up caller doesn't hold the goroutine.
func (m *Manager) executeDelay(session *CallSession, node *IVRNode) string {
	seconds := getConfigFloat(node.Config, "seconds", 2)
	if seconds < 0.5 {
		seconds = 0.5
	}
	if seconds > 60 {
		seconds = 60
	}

	deadline := time.Now().Add(time.Duration(seconds * float64(time.Second)))
	for time.Now().Before(deadline) {
		session.mu.Lock()
		active := session.Status == models.CallStatusAnswered
		session.mu.Unlock()
		if !active {
			return ""
		}
		wait := time.Until(deadline)
		if wait > 200*time.Millisecond {
			wait = 200 * time.Millisecond
		}
		time.Sleep(wait)
	}
	return "default"
}

// executeCondition routes "true" or "false" on an expr-lang expression over the
// call's variables, e.g. `exists == true and http_status == 200`. Values from
// JSON keep their type; everything else (caller_phone, gathered digits) is a
// string. An empty or broken expression counts as false.
func (m *Manager) executeCondition(session *CallSession, node *IVRNode, ctx *IVRContext) string {
	expression, _ := node.Config["expression"].(string)
	if strings.TrimSpace(expression) == "" {
		m.log.Warn("Condition node missing expression", "call_id", session.ID, "node_id", node.ID)
		return "false"
	}

	matched, err := flowgraph.EvaluateCondition(expression, conditionEnv(ctx))
	if err != nil {
		m.log.Warn("Condition expression failed", "call_id", session.ID, "node_id", node.ID, "expression", expression, "error", err)
		return "false"
	}
	m.log.Info("Condition evaluated", "call_id", session.ID, "node_id", node.ID, "result", matched)
	if matched {
		return "true"
	}
	return "false"
}

// conditionEnv merges the call's string variables with their typed values.
func conditionEnv(ctx *IVRContext) map[string]any {
	env := make(map[string]any, len(ctx.Variables)+len(ctx.Values))
	for k, v := range ctx.Variables {
		env[k] = v
	}
	for k, v := range ctx.Values {
		env[k] = v
	}
	return env
}

// getConfigFloat extracts a number from a config map with a default fallback.
func getConfigFloat(config map[string]any, key string, defaultVal float64) float64 {
	switch n := config[key].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f
		}
	}
	return defaultVal
}
