package handlers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/zerodha/logf"
	"gorm.io/gorm"

	"github.com/shridarpatil/whatomate/internal/models"
)

// BackfillChatbotFlowGraph fills ChatbotFlow.Graph for every row where
// it is currently NULL by running stepsToGraph against the legacy
// chatbot_flow_steps table + canvas_layout column. Idempotent and safe
// to re-run.
//
// Called from main.go after RunMigrationWithProgress. Once every flow
// has Graph populated the dispatcher uses the v2 runner unconditionally.
// The legacy table and canvas_layout column are not dropped — they're
// quarantined as dead data until a future maintenance migration.
//
// The migration reads the legacy tables via raw queries so this package
// no longer needs Steps[] / CanvasLayout fields on the ChatbotFlow
// struct.
// Returns the number of flows converted so the caller can drop the
// chatbot flow Redis cache — it survives restarts and would otherwise
// keep serving graph-less copies until its TTL expires.
func BackfillChatbotFlowGraph(db *gorm.DB, lo logf.Logger) (int, error) {
	// Fresh installs never had the legacy schema — chatbot_flow_steps does
	// not exist. Skip in that case so a clean boot doesn't fail.
	//
	// canvas_layout is optional: forks that stored node positions elsewhere
	// (or DBs where the column was already dropped) still have convertible
	// steps, they just fall back to auto-layout positions.
	mig := db.Migrator()
	if !mig.HasTable("chatbot_flow_steps") {
		return 0, nil
	}
	hasCanvasLayout := mig.HasColumn(&models.ChatbotFlow{}, "canvas_layout")

	layoutSelect := `'{}'::jsonb AS canvas_layout`
	if hasCanvasLayout {
		layoutSelect = `COALESCE(canvas_layout, '{}'::jsonb) AS canvas_layout`
	}

	var pending []legacyFlowMeta
	err := db.Raw(fmt.Sprintf(`
		SELECT id, name, %s
		FROM chatbot_flows
		WHERE (graph IS NULL OR graph::text = '{}')
		  AND deleted_at IS NULL
	`, layoutSelect)).Scan(&pending).Error
	if err != nil {
		return 0, fmt.Errorf("load flows for graph backfill: %w", err)
	}

	if len(pending) == 0 {
		return 0, nil
	}

	converted, skipped := 0, 0
	for _, p := range pending {
		var steps []models.ChatbotFlowStep
		if err := db.Where("flow_id = ?", p.ID).Find(&steps).Error; err != nil {
			return converted, fmt.Errorf("load steps for flow %s: %w", p.ID, err)
		}

		graph := stepsToGraph(steps, p.CanvasLayout)
		if graph == nil {
			skipped++
			lo.Warn("Chatbot flow graph backfill: skipping flow with no v2 mapping",
				"flow_id", p.ID, "name", p.Name, "step_count", len(steps))
			continue
		}
		if err := db.Model(&models.ChatbotFlow{}).Where("id = ?", p.ID).Update("graph", graph).Error; err != nil {
			return converted, fmt.Errorf("save backfilled graph for flow %s: %w", p.ID, err)
		}
		converted++
	}

	lo.Info("Chatbot flow graph backfill complete",
		"converted", converted, "skipped", skipped, "total", len(pending))
	return converted, nil
}

type legacyFlowMeta struct {
	ID           uuid.UUID
	Name         string
	CanvasLayout models.JSONB `gorm:"column:canvas_layout"`
}

// stepsToGraph converts a legacy slice of ChatbotFlowStep rows plus an
// optional canvas_layout into a v2 graph JSONB blob, mirroring the
// TypeScript converter in frontend/src/composables/useChatbotFlowConverter.ts.
//
// Returns nil if there are no steps or any step uses a message_type
// that has no v2 mapping.
//
// Legacy steps route to the "__end__" sentinel rather than a real step, so
// a synthetic end node is appended whenever any edge targets it. The node
// carries no message of its own — the flow-level completion message is sent
// by the runner on completion, exactly as the v1 engine did.
func stepsToGraph(steps []models.ChatbotFlowStep, canvasLayout models.JSONB) models.JSONB {
	if len(steps) == 0 {
		return nil
	}

	for i := range steps {
		if !v2SupportedMessageType(steps[i].MessageType) {
			return nil
		}
	}

	sorted := make([]models.ChatbotFlowStep, len(steps))
	copy(sorted, steps)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].StepOrder < sorted[j].StepOrder
	})

	positions := extractCanvasPositions(canvasLayout)

	nodes := make([]map[string]any, 0, len(sorted))
	stepNames := make(map[string]struct{}, len(sorted))
	for _, s := range sorted {
		stepNames[s.StepName] = struct{}{}
	}
	// "__end__" is a v1 sentinel target, not a step row. Whitelist it so
	// buildEdges keeps those edges; the matching node is appended below.
	stepNames[legacyEndSentinel] = struct{}{}

	for i, step := range sorted {
		// v1 text steps that capture a reply become v2 prompt nodes
		// (blocking + validating + storing), matching the editor's UX
		// intent. Trigger on an explicit input_type OR on store_as /
		// validation being configured — a step that only set store_as
		// (no input_type) still needs to capture its reply rather than
		// being flattened to a plain message that drops {{var}}.
		nodeType := messageTypeToNodeTypeGo(string(step.MessageType))
		if step.MessageType == models.FlowStepTypeText && textStepCapturesInput(&step) {
			nodeType = "prompt"
		}

		pos := positions[step.StepName]
		if pos == nil {
			pos = map[string]float64{"x": 300, "y": float64(i * 150)}
		}

		config := buildNodeConfig(nodeType, &step)

		nodes = append(nodes, map[string]any{
			"id":       step.StepName,
			"type":     nodeType,
			"label":    step.StepName,
			"position": map[string]any{"x": pos["x"], "y": pos["y"]},
			"config":   config,
		})
	}

	edges := buildEdges(sorted, stepNames)

	// Materialize the "__end__" sentinel as a real end node when anything
	// routes to it.
	if edgesTarget(edges, legacyEndSentinel) {
		nodes = append(nodes, map[string]any{
			"id":       legacyEndSentinel,
			"type":     "end",
			"label":    "End",
			"position": map[string]any{"x": float64(300), "y": float64(len(sorted) * 150)},
			"config":   map[string]any{},
		})
	}

	// Prepend a fixed `start` sentinel node so the editor always has a
	// non-deletable entry point. The first ordered step becomes the
	// target of start's default edge.
	const startID = "__start__"
	if len(sorted) > 0 {
		nodes = append([]map[string]any{{
			"id":       startID,
			"type":     "start",
			"label":    "Start",
			"position": map[string]any{"x": 100, "y": 100},
			"config":   map[string]any{},
		}}, nodes...)
		edges = append([]map[string]any{{
			"from":      startID,
			"to":        sorted[0].StepName,
			"condition": "default",
		}}, edges...)
	}

	return models.JSONB{
		"version":    2,
		"nodes":      nodes,
		"edges":      edges,
		"entry_node": startID,
	}
}

// legacyEndSentinel is the v1 next_step / conditional_next value meaning
// "finish the flow here". It has no matching row in chatbot_flow_steps.
const legacyEndSentinel = "__end__"

// v2SupportedMessageType keeps the list of v1 → v2 mappings in lockstep
// with the TS V2_SUPPORTED_MESSAGE_TYPES set. "branch" is this fork's v1
// condition step (input_config {variable, operator, value} +
// conditional_next {true, false}) and maps onto the v2 condition node.
func v2SupportedMessageType(mt models.FlowStepType) bool {
	switch string(mt) {
	case "text", "buttons", "end", "condition", "branch", "timing", "goto_flow", "api_fetch", "whatsapp_flow", "transfer":
		return true
	}
	return false
}

// edgesTarget reports whether any edge points at the given node id.
func edgesTarget(edges []map[string]any, target string) bool {
	for _, e := range edges {
		if to, _ := e["to"].(string); to == target {
			return true
		}
	}
	return false
}

// textStepCapturesInput reports whether a legacy text step expected and
// stored a user reply. Such steps map to a v2 prompt (which blocks + stores)
// rather than a plain message (which only sends), so {{var}} interpolation
// keeps working after migration.
func textStepCapturesInput(step *models.ChatbotFlowStep) bool {
	if step.InputType != "" && step.InputType != models.InputTypeNone {
		return true
	}
	return step.StoreAs != "" || step.ValidationRegex != ""
}

func messageTypeToNodeTypeGo(messageType string) string {
	switch messageType {
	case "text":
		return "message"
	case "buttons":
		return "buttons"
	case "end":
		return "end"
	case "condition", "branch":
		return "condition"
	case "timing":
		return "timing"
	case "goto_flow":
		return "goto_flow"
	case "api_fetch":
		return "api_call"
	case "whatsapp_flow":
		return "whatsapp_flow"
	case "transfer":
		return "transfer"
	}
	return messageType
}

// extractCanvasPositions reads { node_positions: { name: {x, y} } } out
// of the legacy CanvasLayout JSONB.
func extractCanvasPositions(raw models.JSONB) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	if raw == nil {
		return out
	}
	positions, ok := raw["node_positions"].(map[string]any)
	if !ok {
		return out
	}
	for name, posAny := range positions {
		posMap, ok := posAny.(map[string]any)
		if !ok {
			continue
		}
		x, _ := posMap["x"].(float64)
		y, _ := posMap["y"].(float64)
		out[name] = map[string]float64{"x": x, "y": y}
	}
	return out
}

func buildNodeConfig(nodeType string, step *models.ChatbotFlowStep) map[string]any {
	config := map[string]any{}
	switch nodeType {
	case "message":
		config["message"] = step.Message
	case "prompt":
		config["body"] = step.Message
		// v1 typed inputs (location in particular) need special capture
		// handling in the runner, so carry the type onto the node.
		if step.InputType != "" && step.InputType != models.InputTypeNone {
			config["input_type"] = string(step.InputType)
		}
		if step.ValidationRegex != "" {
			config["validation_regex"] = step.ValidationRegex
		}
		if step.ValidationError != "" {
			config["validation_error"] = step.ValidationError
		}
		if step.StoreAs != "" {
			config["store_as"] = step.StoreAs
		}
		if step.MaxRetries > 0 {
			config["max_retries"] = step.MaxRetries
		}
	case "buttons":
		config["body"] = step.Message
		config["buttons"] = jsonbArrayToSlice(step.Buttons)
		if step.StoreAs != "" {
			config["store_as"] = step.StoreAs
		}
	case "end":
		if step.Message != "" {
			config["message"] = step.Message
		}
	case "condition":
		expression, _ := getStringFromJSONB(step.InputConfig, "expression")
		if expression == "" {
			// This fork's v1 `branch` step stored a structured comparison
			// instead of an expression — translate it.
			expression = legacyBranchExpression(step.InputConfig)
		}
		config["expression"] = expression
	case "timing":
		if schedule, ok := step.InputConfig["schedule"].([]any); ok {
			config["schedule"] = schedule
		} else {
			config["schedule"] = []any{}
		}
	case "goto_flow":
		fid, _ := getStringFromJSONB(step.InputConfig, "flow_id")
		config["flow_id"] = fid
	case "api_call":
		ac := step.ApiConfig
		config["url"], _ = getStringFromJSONB(ac, "url")
		method, _ := getStringFromJSONB(ac, "method")
		if method == "" {
			method = "GET"
		}
		config["method"] = method
		if headers, ok := ac["headers"].(map[string]any); ok {
			config["headers"] = headers
		} else {
			config["headers"] = map[string]any{}
		}
		body, _ := getStringFromJSONB(ac, "body")
		config["body"] = body
		if mapping, ok := ac["response_mapping"].(map[string]any); ok {
			config["response_mapping"] = mapping
		} else {
			config["response_mapping"] = map[string]any{}
		}
		if fb, _ := getStringFromJSONB(ac, "fallback_message"); fb != "" {
			config["fallback_message"] = fb
		}
		if step.Message != "" {
			config["message_template"] = step.Message
		}
	case "whatsapp_flow":
		ic := step.InputConfig
		flowID, _ := getStringFromJSONB(ic, "whatsapp_flow_id")
		if flowID == "" {
			flowID, _ = getStringFromJSONB(ic, "flow_id")
		}
		config["flow_id"] = flowID
		header, _ := getStringFromJSONB(ic, "flow_header")
		if header == "" {
			header, _ = getStringFromJSONB(ic, "header")
		}
		config["header"] = header
		cta, _ := getStringFromJSONB(ic, "flow_cta")
		if cta == "" {
			cta, _ = getStringFromJSONB(ic, "cta")
		}
		config["cta"] = cta
		if step.Message != "" {
			config["body"] = step.Message
		}
	case "transfer":
		tc := step.TransferConfig
		if teamID, _ := getStringFromJSONB(tc, "team_id"); teamID != "" {
			config["team_id"] = teamID
		}
		if notes, _ := getStringFromJSONB(tc, "notes"); notes != "" {
			config["notes"] = notes
		}
		if step.Message != "" {
			config["body"] = step.Message
		}
	}
	return config
}

// legacyBranchExpression translates a v1 branch step's input_config —
// {"variable": "userExists", "operator": "equals", "value": "true"} — into
// an expr-lang expression evaluated by execChatCondition.
//
// It mirrors the deleted evaluateBranchCondition: the variable is coerced
// to a string, comparisons are case-insensitive, and an empty/unknown
// operator means "is the variable truthy". Missing variables coerce to ""
// via `?? ""` so the expression never errors out at runtime.
//
// Returns "" when no variable is configured — execChatCondition logs that
// and routes to the "false" branch, matching v1's completeFlow-on-missing
// -variable behavior closely enough to be debuggable.
func legacyBranchExpression(inputConfig models.JSONB) string {
	variable, _ := getStringFromJSONB(inputConfig, "variable")
	if variable == "" {
		return ""
	}
	operator, _ := getStringFromJSONB(inputConfig, "operator")
	value, _ := getStringFromJSONB(inputConfig, "value")

	// `v` is the raw value (nil when unset), `s` its string coercion.
	v := variable
	s := fmt.Sprintf("string(%s ?? \"\")", variable)
	quoted := strconv.Quote(value)
	lowered := strconv.Quote(strings.ToLower(value))

	switch operator {
	case "equals", "eq", "==":
		return fmt.Sprintf("lower(%s) == %s", s, lowered)
	case "not_equals", "neq", "!=":
		return fmt.Sprintf("lower(%s) != %s", s, lowered)
	case "exists":
		return fmt.Sprintf("%s != nil", v)
	case "not_exists":
		return fmt.Sprintf("%s == nil", v)
	case "not_empty":
		return fmt.Sprintf("%s != nil && %s != \"\"", v, s)
	case "empty":
		return fmt.Sprintf("%s == nil || %s == \"\"", v, s)
	case "contains":
		return fmt.Sprintf("indexOf(lower(%s), %s) != -1", s, lowered)
	case "gt", ">":
		return fmt.Sprintf("%s > %s", s, quoted)
	case "lt", "<":
		return fmt.Sprintf("%s < %s", s, quoted)
	default:
		// v1 default: truthiness of the variable.
		return fmt.Sprintf("%s != nil && %s != \"\" && lower(%s) != \"false\" && %s != \"0\"", v, s, s, s)
	}
}

func buildEdges(sorted []models.ChatbotFlowStep, stepNames map[string]struct{}) []map[string]any {
	edges := make([]map[string]any, 0)

	for i, step := range sorted {
		var nextSequential string
		if i < len(sorted)-1 {
			nextSequential = sorted[i+1].StepName
		}

		mt := string(step.MessageType)
		switch mt {
		case "buttons":
			edges = append(edges, buttonsEdges(step, nextSequential, stepNames)...)
		case "condition", "branch":
			edges = append(edges, branchEdges(step, []string{"true", "false"}, stepNames)...)
		case "timing":
			edges = append(edges, branchEdges(step, []string{"in_hours", "out_of_hours"}, stepNames)...)
		case "transfer", "end", "goto_flow":
			// Terminal — no outgoing edges.
		default:
			// message / api_fetch / whatsapp_flow — sequential fallthrough.
			target := step.NextStep
			if target == "" {
				target = nextSequential
			}
			if target != "" {
				if _, ok := stepNames[target]; ok {
					edges = append(edges, map[string]any{
						"from": step.StepName, "to": target, "condition": "default",
					})
				}
			}
		}
	}
	return edges
}

func buttonsEdges(step models.ChatbotFlowStep, nextSequential string, stepNames map[string]struct{}) []map[string]any {
	edges := make([]map[string]any, 0)
	mapped := map[string]struct{}{}

	for buttonID, targetAny := range step.ConditionalNext {
		target, _ := targetAny.(string)
		if target == "" {
			continue
		}
		mapped[buttonID] = struct{}{}
		if _, ok := stepNames[target]; ok {
			edges = append(edges, map[string]any{
				"from": step.StepName, "to": target, "condition": "button:" + buttonID,
			})
		}
	}

	// Unmapped buttons fall through to the next sequential step.
	if nextSequential != "" {
		if _, ok := stepNames[nextSequential]; ok {
			for _, btn := range step.Buttons {
				btnMap, ok := btn.(map[string]any)
				if !ok {
					continue
				}
				id, _ := btnMap["id"].(string)
				if id == "" {
					continue
				}
				if _, already := mapped[id]; already {
					continue
				}
				edges = append(edges, map[string]any{
					"from": step.StepName, "to": nextSequential, "condition": "button:" + id,
				})
			}
		}
	}
	return edges
}

func branchEdges(step models.ChatbotFlowStep, handles []string, stepNames map[string]struct{}) []map[string]any {
	edges := make([]map[string]any, 0, len(handles))
	for _, h := range handles {
		target, _ := step.ConditionalNext[h].(string)
		if target == "" {
			continue
		}
		if _, ok := stepNames[target]; ok {
			edges = append(edges, map[string]any{
				"from": step.StepName, "to": target, "condition": h,
			})
		}
	}
	return edges
}

func jsonbArrayToSlice(arr models.JSONBArray) []any {
	out := make([]any, 0, len(arr))
	out = append(out, arr...)
	return out
}

func getStringFromJSONB(j models.JSONB, key string) (string, bool) {
	if j == nil {
		return "", false
	}
	if v, ok := j[key].(string); ok {
		return v, true
	}
	return "", false
}
