package calling

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/flowgraph"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

// fakeSynth records the text it was asked to speak.
type fakeSynth struct {
	spoken []string
}

func (f *fakeSynth) Generate(text string) (string, error) {
	f.spoken = append(f.spoken, text)
	return "tts_fake.ogg", nil
}

func newDynamicTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		config: &config.CallingConfig{AudioDir: t.TempDir()},
		log:    logf.New(logf.Opts{}),
	}
}

func answeredSession() *CallSession {
	return &CallSession{
		ID:         "wacid.test",
		Status:     models.CallStatusAnswered,
		DTMFBuffer: make(chan byte, 8),
	}
}

func TestLookupJSONPath(t *testing.T) {
	var data any = map[string]any{
		"prompt": "Press 1",
		"data": map[string]any{
			"slots": []any{
				map[string]any{"id": "a", "start": "16:00"},
				map[string]any{"id": "b"},
			},
		},
		"count": float64(2),
	}

	tests := []struct {
		path string
		want any
		ok   bool
	}{
		{"prompt", "Press 1", true},
		{"data.slots[1].id", "b", true},
		{"data.slots.length", float64(2), true},
		{"count", float64(2), true},
		{"data.slots[5].id", nil, false},
		{"data.missing", nil, false},
		{"prompt.deeper", nil, false},
		{"data.slots[x]", nil, false},
	}
	for _, tt := range tests {
		got, ok := lookupJSONPath(data, tt.path)
		assert.Equal(t, tt.ok, ok, tt.path)
		if tt.ok {
			assert.Equal(t, tt.want, got, tt.path)
		}
	}

	root, ok := lookupJSONPath([]any{"x", "y"}, "[1]")
	require.True(t, ok)
	assert.Equal(t, "y", root)
}

func TestApplyResponseMapping(t *testing.T) {
	ctx := &IVRContext{Variables: map[string]string{"caller_phone": "9198"}, Values: map[string]any{}}
	vars := ctx.Variables
	body := `{"prompt":"Press 1 for 4 PM","options":[{"label":"4 PM","value":"s1"}],"ok":true,"n":3}`

	n, err := applyResponseMapping(body, map[string]any{
		"slots_prompt": "prompt",
		"slots":        "options",
		"first_value":  "options[0].value",
		"ok":           "ok",
		"n":            "n",
		"absent":       "nope",
	}, ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "Press 1 for 4 PM", vars["slots_prompt"])
	assert.JSONEq(t, `[{"label":"4 PM","value":"s1"}]`, vars["slots"])
	assert.Equal(t, "s1", vars["first_value"])
	assert.Equal(t, "true", vars["ok"])
	assert.Equal(t, "3", vars["n"])
	assert.NotContains(t, vars, "absent")
	assert.Equal(t, "9198", vars["caller_phone"], "existing variables are kept")
	assert.Equal(t, true, ctx.Values["ok"], "typed value kept for conditions")
	assert.Equal(t, float64(3), ctx.Values["n"])

	_, err = applyResponseMapping("not json", map[string]any{"x": "y"}, ctx)
	assert.Error(t, err)
}

func TestParseDynamicOptions(t *testing.T) {
	t.Run("objects without digits are numbered in order", func(t *testing.T) {
		opts, err := parseDynamicOptions(`[{"label":"4 to 5","value":"s1"},{"label":"7 to 8:30","value":"s2"}]`)
		require.NoError(t, err)
		require.Len(t, opts, 2)
		assert.Equal(t, dynamicMenuOption{Digit: "1", Label: "4 to 5", Value: "s1"}, opts[0])
		assert.Equal(t, dynamicMenuOption{Digit: "2", Label: "7 to 8:30", Value: "s2"}, opts[1])
	})

	t.Run("explicit digits are kept and auto-numbering skips them", func(t *testing.T) {
		opts, err := parseDynamicOptions(`[{"digit":"1","label":"A"},{"label":"B"},{"digit":"9","label":"C"}]`)
		require.NoError(t, err)
		require.Len(t, opts, 3)
		assert.Equal(t, "1", opts[0].Digit)
		assert.Equal(t, "2", opts[1].Digit)
		assert.Equal(t, "9", opts[2].Digit)
		assert.Equal(t, "A", opts[0].Value, "value falls back to label")
	})

	t.Run("plain strings", func(t *testing.T) {
		opts, err := parseDynamicOptions(`["slot_a","slot_b"]`)
		require.NoError(t, err)
		require.Len(t, opts, 2)
		assert.Equal(t, "slot_b", opts[1].Value)
	})

	t.Run("capped at nine and invalid digits dropped", func(t *testing.T) {
		opts, err := parseDynamicOptions(`[1,2,3,4,5,6,7,8,9,10,11]`)
		require.NoError(t, err)
		assert.Len(t, opts, 9)

		opts, err = parseDynamicOptions(`[{"digit":"12","label":"x"},{"digit":"1","label":"y"},{"digit":"1","label":"dup"}]`)
		require.NoError(t, err)
		require.Len(t, opts, 1)
		assert.Equal(t, "y", opts[0].Label)
	})

	t.Run("empty and invalid input", func(t *testing.T) {
		opts, err := parseDynamicOptions("")
		require.NoError(t, err)
		assert.Empty(t, opts)

		opts, err = parseDynamicOptions(`[]`)
		require.NoError(t, err)
		assert.Empty(t, opts)

		_, err = parseDynamicOptions(`{"not":"array"}`)
		assert.Error(t, err)
	})
}

func TestPromptAudioPath(t *testing.T) {
	m := newDynamicTestManager(t)
	session := answeredSession()
	ctx := &IVRContext{Variables: map[string]string{"slots_prompt": "Press 1 for 4 PM"}}

	t.Run("static text uses the file prepared at save time", func(t *testing.T) {
		synth := &fakeSynth{}
		m.SetTTS(synth)
		node := &IVRNode{ID: "g", Config: map[string]any{"greeting_text": "Welcome", "audio_file": "tts_welcome.ogg"}}
		assert.Equal(t, filepath.Join(m.config.AudioDir, "tts_welcome.ogg"), m.promptAudioPath(session, node, ctx))
		assert.Empty(t, synth.spoken)
	})

	t.Run("text with variables is filled in and spoken live", func(t *testing.T) {
		synth := &fakeSynth{}
		m.SetTTS(synth)
		node := &IVRNode{ID: "g", Config: map[string]any{"greeting_text": "Hi. {{slots_prompt}}"}}
		assert.Equal(t, filepath.Join(m.config.AudioDir, "tts_fake.ogg"), m.promptAudioPath(session, node, ctx))
		assert.Equal(t, []string{"Hi. Press 1 for 4 PM"}, synth.spoken)
	})

	t.Run("text that resolves to nothing plays nothing", func(t *testing.T) {
		synth := &fakeSynth{}
		m.SetTTS(synth)
		node := &IVRNode{ID: "g", Config: map[string]any{"greeting_text": "{{unset}}"}}
		ctx := &IVRContext{Variables: map[string]string{"unset": ""}}
		assert.Equal(t, "", m.promptAudioPath(session, node, ctx))
		assert.Empty(t, synth.spoken)
	})

	t.Run("without TTS a variable prompt falls back to its audio file", func(t *testing.T) {
		m.SetTTS(nil)
		node := &IVRNode{ID: "g", Config: map[string]any{"greeting_text": "{{slots_prompt}}", "audio_file": "fallback.ogg"}}
		assert.Equal(t, filepath.Join(m.config.AudioDir, "fallback.ogg"), m.promptAudioPath(session, node, ctx))
	})
}

func TestExecuteDynamicMenu(t *testing.T) {
	options := `[{"label":"4 to 5 PM","value":"slot_a"},{"label":"7 to 8:30 PM","value":"slot_b"}]`
	node := &IVRNode{ID: "dm", Type: IVRNodeMenu, Config: map[string]any{
		"dynamic_options":  true,
		"options_variable": "slots",
		"store_as":         "slot_id",
		"timeout_seconds":  float64(1),
		"max_retries":      float64(2),
	}}

	t.Run("valid key selects the option", func(t *testing.T) {
		m := newDynamicTestManager(t)
		session := answeredSession()
		ctx := &IVRContext{Variables: map[string]string{"slots": options}}
		go func() {
			time.Sleep(100 * time.Millisecond)
			session.DTMFBuffer <- '2'
		}()

		// No prompt audio, so the player is never touched.
		assert.Equal(t, "selected", m.executeDynamicMenu(session, node, ctx, nil))
		assert.Equal(t, "2", ctx.Variables["last_menu_digit"])
		assert.Equal(t, "slot_b", ctx.Variables["selected_option_value"])
		assert.Equal(t, "7 to 8:30 PM", ctx.Variables["selected_option_label"])
		assert.Equal(t, "slot_b", ctx.Variables["slot_id"])
	})

	t.Run("key outside the options is rejected until retries run out", func(t *testing.T) {
		m := newDynamicTestManager(t)
		session := answeredSession()
		ctx := &IVRContext{Variables: map[string]string{"slots": options}}
		// promptForDigit drains the buffer before each attempt, so feed the
		// out-of-range key once the first wait has started.
		go func() {
			time.Sleep(100 * time.Millisecond)
			session.DTMFBuffer <- '3'
		}()

		assert.Equal(t, "max_retries", m.executeDynamicMenu(session, node, ctx, nil))
		assert.NotContains(t, ctx.Variables, "selected_option_value")
	})

	t.Run("no options skips the prompt", func(t *testing.T) {
		m := newDynamicTestManager(t)
		for _, raw := range []string{"", "[]", "garbage"} {
			ctx := &IVRContext{Variables: map[string]string{"slots": raw}}
			assert.Equal(t, "no_options", m.executeDynamicMenu(answeredSession(), node, ctx, nil), raw)
		}
	})
}

func TestExecuteDelay(t *testing.T) {
	m := newDynamicTestManager(t)

	t.Run("waits the configured time", func(t *testing.T) {
		start := time.Now()
		outcome := m.executeDelay(answeredSession(), &IVRNode{Config: map[string]any{"seconds": float64(0.5)}})
		assert.Equal(t, "default", outcome)
		assert.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond)
	})

	t.Run("stops early when the call ends", func(t *testing.T) {
		session := answeredSession()
		go func() {
			time.Sleep(150 * time.Millisecond)
			session.mu.Lock()
			session.Status = models.CallStatusCompleted
			session.mu.Unlock()
		}()
		start := time.Now()
		outcome := m.executeDelay(session, &IVRNode{Config: map[string]any{"seconds": float64(30)}})
		assert.Equal(t, "", outcome)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("config values", func(t *testing.T) {
		assert.Equal(t, 2.0, getConfigFloat(map[string]any{}, "seconds", 2))
		assert.Equal(t, 3.0, getConfigFloat(map[string]any{"seconds": "3"}, "seconds", 2))
		assert.Equal(t, 5.0, getConfigFloat(map[string]any{"seconds": 5}, "seconds", 2))
	})
}

func TestExecuteCondition(t *testing.T) {
	m := newDynamicTestManager(t)
	session := answeredSession()

	// What an HTTP "check user" node leaves behind.
	newCtx := func(body string) *IVRContext {
		ctx := &IVRContext{
			Variables: map[string]string{"caller_phone": "919800000001", "http_status": "200"},
			Values:    map[string]any{"http_status": 200},
		}
		_, err := applyResponseMapping(body, map[string]any{"exists": "exists", "name": "user.name", "orders": "user.orders"}, ctx)
		require.NoError(t, err)
		return ctx
	}
	cond := func(expression string) *IVRNode {
		return &IVRNode{ID: "c", Type: IVRNodeCondition, Config: map[string]any{"expression": expression}}
	}

	existing := newCtx(`{"exists": true, "user": {"name": "Rahul", "orders": 4}}`)
	unknown := newCtx(`{"exists": false}`)

	assert.Equal(t, "true", m.executeCondition(session, cond("exists == true"), existing))
	assert.Equal(t, "false", m.executeCondition(session, cond("exists == true"), unknown))
	assert.Equal(t, "true", m.executeCondition(session, cond("exists"), existing), "bare boolean")
	assert.Equal(t, "true", m.executeCondition(session, cond(`http_status == 200 and orders > 3 and name == "Rahul"`), existing))
	assert.Equal(t, "true", m.executeCondition(session, cond(`caller_phone startsWith "9198"`), existing), "plain variables are strings")
	assert.Equal(t, "false", m.executeCondition(session, cond("orders > 0"), unknown), "unset variable is nil, not an error")
	assert.Equal(t, "false", m.executeCondition(session, cond(""), existing), "empty expression")
	assert.Equal(t, "false", m.executeCondition(session, cond("exists ==="), existing), "broken expression")
}

func TestFlowgraphLocation(t *testing.T) {
	assert.Equal(t, "Asia/Kolkata", flowgraph.Location("Asia/Kolkata").String())
	assert.Equal(t, "Asia/Kolkata", flowgraph.Location("", "Not/AZone", " Asia/Kolkata ").String())
	assert.Equal(t, time.Local, flowgraph.Location("", "Not/AZone"))
}

// The org's timezone decides business hours, not the server's: 10:00 IST is
// 04:30 UTC, outside a 09:00–17:00 day read in UTC.
func TestExecuteTiming_UsesOrgTimezone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	m := &Manager{config: &config.CallingConfig{}, log: logf.New(logf.Opts{}), db: db}
	session := &CallSession{ID: "wacid.t", OrganizationID: org.ID}

	ist := flowgraph.Location("Asia/Kolkata")
	nowIST := time.Now().In(ist)
	// A one-hour window around the current IST time, on today's IST weekday.
	start := nowIST.Add(-30 * time.Minute)
	end := nowIST.Add(30 * time.Minute)
	if start.Day() != nowIST.Day() || end.Day() != nowIST.Day() {
		t.Skip("window would straddle IST midnight")
	}
	node := &IVRNode{ID: "t", Type: IVRNodeTiming, Config: map[string]any{"schedule": []any{
		map[string]any{
			"day": strings.ToLower(nowIST.Weekday().String()), "enabled": true,
			"start_time": start.Format("15:04"), "end_time": end.Format("15:04"),
		},
	}}}

	require.NoError(t, db.Exec(`UPDATE organizations SET settings = '{"timezone":"Asia/Kolkata"}' WHERE id = ?`, org.ID).Error)
	assert.Equal(t, "in_hours", m.executeTiming(session, node))

	// Same window read in a zone 12+ hours away is out of hours.
	require.NoError(t, db.Exec(`UPDATE organizations SET settings = '{"timezone":"America/Los_Angeles"}' WHERE id = ?`, org.ID).Error)
	assert.Equal(t, "out_of_hours", m.executeTiming(session, node))
}

func TestPromptForDigit_TimeoutVersusInvalidKeys(t *testing.T) {
	node := &IVRNode{ID: "m", Type: IVRNodeMenu, Config: map[string]any{
		"options":         map[string]any{"1": map[string]any{"label": "A"}},
		"timeout_seconds": float64(1),
		"max_retries":     float64(1),
	}}
	ctx := func() *IVRContext { return &IVRContext{Variables: map[string]string{}} }

	t.Run("no key at all is a timeout", func(t *testing.T) {
		m := newDynamicTestManager(t)
		assert.Equal(t, "timeout", m.executeMenu(answeredSession(), node, ctx(), nil))
	})

	t.Run("only invalid keys is max_retries", func(t *testing.T) {
		m := newDynamicTestManager(t)
		session := answeredSession()
		go func() {
			time.Sleep(100 * time.Millisecond)
			session.DTMFBuffer <- '7'
		}()
		assert.Equal(t, "max_retries", m.executeMenu(session, node, ctx(), nil))
	})
}

// A flow saved before "timeout" existed wires silence to max_retries.
func TestNodeLoop_TimeoutFallsBackToMaxRetriesEdge(t *testing.T) {
	g := &IVRFlowGraph{Version: 2, EntryNode: "m", Nodes: []IVRNode{
		{ID: "m", Type: IVRNodeMenu}, {ID: "bye", Type: IVRNodeHangup}, {ID: "to", Type: IVRNodeHangup},
	}, Edges: []IVREdge{{From: "m", To: "bye", Condition: "max_retries"}}}
	g.BuildMaps()
	assert.Equal(t, "", g.ResolveEdge("m", "timeout"), "no timeout edge")

	g.Edges = append(g.Edges, IVREdge{From: "m", To: "to", Condition: "timeout"})
	g.BuildMaps()
	assert.Equal(t, "to", g.ResolveEdge("m", "timeout"), "an explicit timeout edge wins")
}

func TestSaveIVRPath_StoresVariables(t *testing.T) {
	db := testutil.SetupTestDB(t)
	org := testutil.CreateTestOrganization(t, db)
	account := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	contact := testutil.CreateTestContact(t, db, org.ID)
	callLog := models.CallLog{OrganizationID: org.ID, WhatsAppAccount: account.Name, ContactID: contact.ID,
		WhatsAppCallID: "wacid.save", CallerPhone: contact.PhoneNumber, Status: models.CallStatusAnswered}
	require.NoError(t, db.Create(&callLog).Error)

	m := &Manager{config: &config.CallingConfig{}, log: logf.New(logf.Opts{}), db: db}
	ctx := &IVRContext{
		Path:      []map[string]string{{"node": "slots", "type": "menu", "outcome": "selected"}},
		Variables: map[string]string{"slot_id": "rps_1@2026-10-19", "booking_message": "Booked for tomorrow."},
	}
	m.saveIVRPath(&CallSession{ID: "wacid.save", CallLogID: callLog.ID}, ctx)

	var saved models.CallLog
	require.NoError(t, db.First(&saved, "id = ?", callLog.ID).Error)
	vars, ok := saved.IVRPath["variables"].(map[string]any)
	require.True(t, ok, "variables saved: %v", saved.IVRPath)
	assert.Equal(t, "rps_1@2026-10-19", vars["slot_id"])
	assert.Equal(t, "Booked for tomorrow.", vars["booking_message"])
	assert.Len(t, saved.IVRPath["steps"], 1)
}
