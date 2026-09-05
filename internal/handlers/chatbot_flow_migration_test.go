package handlers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// graphNodes pulls out the nodes slice from the JSONB output.
func graphNodes(t *testing.T, g models.JSONB) []map[string]any {
	t.Helper()
	require.NotNil(t, g)
	raw, ok := g["nodes"].([]map[string]any)
	require.True(t, ok, "graph.nodes wrong type")
	return raw
}

func graphEdges(t *testing.T, g models.JSONB) []map[string]any {
	t.Helper()
	require.NotNil(t, g)
	raw, ok := g["edges"].([]map[string]any)
	require.True(t, ok, "graph.edges wrong type")
	return raw
}

func TestStepsToGraph_EmptyReturnsNil(t *testing.T) {
	assert.Nil(t, stepsToGraph(nil, nil))
	assert.Nil(t, stepsToGraph([]models.ChatbotFlowStep{}, nil))
}

func TestStepsToGraph_TextStepProducesMessageNode(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{StepName: "step_1", StepOrder: 1, MessageType: "text", Message: "Hi!"},
	}
	g := stepsToGraph(steps, nil)
	require.Equal(t, 2, g["version"])
	// Entry is the injected start sentinel; the legacy step sits at [1].
	assert.Equal(t, "__start__", g["entry_node"])

	nodes := graphNodes(t, g)
	require.Len(t, nodes, 2)
	assert.Equal(t, "start", nodes[0]["type"])
	assert.Equal(t, "message", nodes[1]["type"])
	cfg, _ := nodes[1]["config"].(map[string]any)
	assert.Equal(t, "Hi!", cfg["message"])
}

func TestStepsToGraph_TextStepWithStoreAsBecomesPrompt(t *testing.T) {
	// A legacy text step that captures a reply via store_as (but with no
	// explicit input_type) must migrate to a prompt node that carries the
	// store_as, not a plain message that drops it.
	steps := []models.ChatbotFlowStep{
		{StepName: "ask_name", StepOrder: 1, MessageType: "text", Message: "Your name?", StoreAs: "name"},
	}
	nodes := graphNodes(t, stepsToGraph(steps, nil))
	require.Len(t, nodes, 2)
	assert.Equal(t, "prompt", nodes[1]["type"])
	cfg, _ := nodes[1]["config"].(map[string]any)
	assert.Equal(t, "Your name?", cfg["body"])
	assert.Equal(t, "name", cfg["store_as"])
}

// This fork's v1 "branch" step has no expression — it stores a structured
// {variable, operator, value} comparison — and must land on a condition
// node with an equivalent expr-lang expression plus true/false edges.
func TestStepsToGraph_BranchStepBecomesConditionNode(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "check", StepOrder: 1, MessageType: "branch",
			InputConfig:     models.JSONB{"variable": "userExists", "operator": "equals", "value": "true"},
			ConditionalNext: models.JSONB{"true": "known", "false": "onboard"},
		},
		{StepName: "known", StepOrder: 2, MessageType: "text", Message: "wb"},
		{StepName: "onboard", StepOrder: 3, MessageType: "text", Message: "hi"},
	}
	g := stepsToGraph(steps, nil)
	nodes := graphNodes(t, g)
	require.Equal(t, "condition", nodes[1]["type"])
	cfg, _ := nodes[1]["config"].(map[string]any)
	expression, _ := cfg["expression"].(string)
	require.NotEmpty(t, expression)

	// The expression must behave like the deleted evaluateBranchCondition.
	got, err := evaluateConditionExpression(expression, models.JSONB{"userExists": true})
	require.NoError(t, err)
	assert.True(t, got)
	got, err = evaluateConditionExpression(expression, models.JSONB{"userExists": "TRUE"})
	require.NoError(t, err)
	assert.True(t, got, "comparison is case-insensitive")
	got, err = evaluateConditionExpression(expression, models.JSONB{"userExists": false})
	require.NoError(t, err)
	assert.False(t, got)
	got, err = evaluateConditionExpression(expression, models.JSONB{})
	require.NoError(t, err)
	assert.False(t, got, "missing variable must not error")

	hasTrue, hasFalse := false, false
	for _, e := range graphEdges(t, g) {
		if e["from"] == "check" && e["condition"] == "true" && e["to"] == "known" {
			hasTrue = true
		}
		if e["from"] == "check" && e["condition"] == "false" && e["to"] == "onboard" {
			hasFalse = true
		}
	}
	assert.True(t, hasTrue && hasFalse)
}

// A branch with no operator falls back to a truthiness test, matching v1.
func TestStepsToGraph_BranchWithoutOperatorIsTruthiness(t *testing.T) {
	expression := legacyBranchExpression(models.JSONB{"variable": "flag", "value": "true"})
	require.NotEmpty(t, expression)
	for _, tc := range []struct {
		val  any
		want bool
	}{
		{true, true}, {"yes", true}, {1.0, true},
		{false, false}, {"", false}, {0.0, false}, {nil, false},
	} {
		got, err := evaluateConditionExpression(expression, models.JSONB{"flag": tc.val})
		require.NoError(t, err, "value %v", tc.val)
		assert.Equal(t, tc.want, got, "value %v", tc.val)
	}
}

// "__end__" is a v1 sentinel, not a step row: edges pointing at it must
// survive and gain a real end node carrying the completion message.
func TestStepsToGraph_EndSentinelBecomesEndNode(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{StepName: "bye", StepOrder: 1, MessageType: "text", Message: "Bye", NextStep: "__end__"},
	}
	g := stepsToGraph(steps, nil)
	var endNode map[string]any
	for _, n := range graphNodes(t, g) {
		if n["id"] == "__end__" {
			endNode = n
		}
	}
	require.NotNil(t, endNode, "expected a materialized __end__ node")
	assert.Equal(t, "end", endNode["type"])

	found := false
	for _, e := range graphEdges(t, g) {
		if e["from"] == "bye" && e["to"] == "__end__" {
			found = true
		}
	}
	assert.True(t, found, "edge to __end__ must be kept")
}

// Location steps need their input type on the node so the runner captures
// the shared-location payload instead of treating it as free text.
func TestStepsToGraph_LocationStepKeepsInputType(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "where", StepOrder: 1, MessageType: "text", Message: "Share location",
			InputType: models.InputTypeLocation, StoreAs: "location",
		},
	}
	nodes := graphNodes(t, stepsToGraph(steps, nil))
	require.Equal(t, "prompt", nodes[1]["type"])
	cfg, _ := nodes[1]["config"].(map[string]any)
	assert.Equal(t, "location", cfg["input_type"])
	assert.Equal(t, "location", cfg["store_as"])
}

func TestStepsToGraph_ButtonsStepEmitsPerButtonEdges(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "ask", StepOrder: 1, MessageType: "buttons", Message: "Pick",
			Buttons: models.JSONBArray{
				map[string]any{"id": "yes", "title": "Yes"},
				map[string]any{"id": "no", "title": "No"},
			},
			ConditionalNext: models.JSONB{"yes": "thanks", "no": "bye"},
		},
		{StepName: "thanks", StepOrder: 2, MessageType: "text", Message: "Cool"},
		{StepName: "bye", StepOrder: 3, MessageType: "text", Message: "OK"},
	}
	edges := graphEdges(t, stepsToGraph(steps, nil))
	hasYes, hasNo := false, false
	for _, e := range edges {
		if e["from"] == "ask" && e["condition"] == "button:yes" && e["to"] == "thanks" {
			hasYes = true
		}
		if e["from"] == "ask" && e["condition"] == "button:no" && e["to"] == "bye" {
			hasNo = true
		}
	}
	assert.True(t, hasYes && hasNo, "buttons should emit button:<id> edges, got %v", edges)
}

func TestStepsToGraph_ApiFetchToApiCall(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "fetch", StepOrder: 1, MessageType: "api_fetch", Message: "Got {{customer_id}}",
			ApiConfig: models.JSONB{
				"url":              "https://x/api",
				"method":           "POST",
				"headers":          map[string]any{"Auth": "Bearer foo"},
				"body":             `{"a":1}`,
				"response_mapping": map[string]any{"customer_id": "data.id"},
			},
		},
	}
	nodes := graphNodes(t, stepsToGraph(steps, nil))
	require.Len(t, nodes, 2)
	assert.Equal(t, "start", nodes[0]["type"])
	assert.Equal(t, "api_call", nodes[1]["type"])
	cfg, _ := nodes[1]["config"].(map[string]any)
	assert.Equal(t, "https://x/api", cfg["url"])
	assert.Equal(t, "POST", cfg["method"])
	assert.Equal(t, "Got {{customer_id}}", cfg["message_template"])
	rm, _ := cfg["response_mapping"].(map[string]any)
	assert.Equal(t, "data.id", rm["customer_id"])
}

func TestStepsToGraph_WhatsAppFlowFieldRename(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "form", StepOrder: 1, MessageType: "whatsapp_flow", Message: "Open form",
			InputConfig: models.JSONB{
				"whatsapp_flow_id": "meta-1",
				"flow_header":      "Hello",
				"flow_cta":         "Continue",
			},
		},
	}
	cfg, _ := graphNodes(t, stepsToGraph(steps, nil))[1]["config"].(map[string]any)
	assert.Equal(t, "meta-1", cfg["flow_id"])
	assert.Equal(t, "Hello", cfg["header"])
	assert.Equal(t, "Continue", cfg["cta"])
	assert.Equal(t, "Open form", cfg["body"])
}

func TestStepsToGraph_TransferIsTerminal(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "t1", StepOrder: 1, MessageType: "transfer", Message: "Connecting…",
			TransferConfig: models.JSONB{"team_id": "team-uuid", "notes": "n"},
		},
		{StepName: "post", StepOrder: 2, MessageType: "text", Message: "should not connect"},
	}
	for _, e := range graphEdges(t, stepsToGraph(steps, nil)) {
		if e["from"] == "t1" {
			t.Fatalf("transfer should not emit outgoing edges, got %v", e)
		}
	}
}

func TestStepsToGraph_TimingEdgesUseInHoursOutOfHours(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "t1", StepOrder: 1, MessageType: "timing",
			ConditionalNext: models.JSONB{"in_hours": "open", "out_of_hours": "closed"},
			InputConfig:     models.JSONB{"schedule": []any{}},
		},
		{StepName: "open", StepOrder: 2, MessageType: "text", Message: "open"},
		{StepName: "closed", StepOrder: 3, MessageType: "text", Message: "closed"},
	}
	conditions := map[string]string{}
	for _, e := range graphEdges(t, stepsToGraph(steps, nil)) {
		if e["from"] == "t1" {
			conditions[e["condition"].(string)] = e["to"].(string)
		}
	}
	assert.Equal(t, "open", conditions["in_hours"])
	assert.Equal(t, "closed", conditions["out_of_hours"])
}

func TestStepsToGraph_ConditionExpressionPassthrough(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{
			StepName: "c1", StepOrder: 1, MessageType: "condition",
			InputConfig: models.JSONB{"expression": `status == "active"`},
		},
	}
	cfg, _ := graphNodes(t, stepsToGraph(steps, nil))[1]["config"].(map[string]any)
	assert.Equal(t, `status == "active"`, cfg["expression"])
}

func TestStepsToGraph_GotoFlowTerminalWithFlowID(t *testing.T) {
	target := uuid.New()
	steps := []models.ChatbotFlowStep{
		{
			StepName: "g1", StepOrder: 1, MessageType: "goto_flow",
			InputConfig: models.JSONB{"flow_id": target.String()},
		},
	}
	g := stepsToGraph(steps, nil)
	// nodes[0] is the start sentinel; the goto_flow step is at [1].
	cfg, _ := graphNodes(t, g)[1]["config"].(map[string]any)
	assert.Equal(t, target.String(), cfg["flow_id"])
	for _, e := range graphEdges(t, g) {
		if e["from"] == "g1" {
			t.Fatalf("goto_flow should be terminal")
		}
	}
}

func TestStepsToGraph_OrderingFollowsStepOrder(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{StepName: "third", StepOrder: 3, MessageType: "text", Message: "3"},
		{StepName: "first", StepOrder: 1, MessageType: "text", Message: "1"},
		{StepName: "second", StepOrder: 2, MessageType: "text", Message: "2"},
	}
	g := stepsToGraph(steps, nil)
	// entry_node is the injected start sentinel; the original first
	// step sits behind it as start's default-edge target.
	assert.Equal(t, "__start__", g["entry_node"])
	nodes := graphNodes(t, g)
	require.Len(t, nodes, 4)
	assert.Equal(t, "__start__", nodes[0]["id"])
	assert.Equal(t, "first", nodes[1]["id"])
	assert.Equal(t, "second", nodes[2]["id"])
	assert.Equal(t, "third", nodes[3]["id"])
}

func TestStepsToGraph_CanvasPositionsApplied(t *testing.T) {
	steps := []models.ChatbotFlowStep{
		{StepName: "step_1", StepOrder: 1, MessageType: "text", Message: "hi"},
	}
	layout := models.JSONB{
		"node_positions": map[string]any{
			"step_1": map[string]any{"x": 100.0, "y": 200.0},
		},
	}
	// nodes[0] is the start sentinel; the legacy step is at [1].
	pos, _ := graphNodes(t, stepsToGraph(steps, layout))[1]["position"].(map[string]any)
	assert.Equal(t, 100.0, pos["x"])
	assert.Equal(t, 200.0, pos["y"])
}

// TestBackfillChatbotFlowGraph_FillsNullGraphsAndLeavesOthersAlone is
// the integration test: legacy rows (Graph IS NULL with chatbot_flow_steps
// + canvas_layout backing them) get migrated; already-v2 rows are
// untouched; second run is a no-op.
func TestBackfillChatbotFlowGraph_FillsNullGraphsAndLeavesOthersAlone(t *testing.T) {
	app := newProcessorTestApp(t)
	org, account := createProcessorTestOrg(t, app)

	// AutoMigrate no longer creates the legacy canvas_layout column or
	// the chatbot_flow_steps table — the model has moved fully to graph.
	// Recreate just enough of the legacy schema so we can exercise the
	// backfill against representative legacy data.
	require.NoError(t, app.DB.Exec(`ALTER TABLE chatbot_flows ADD COLUMN IF NOT EXISTS canvas_layout JSONB`).Error)
	require.NoError(t, app.DB.AutoMigrate(&models.ChatbotFlowStep{}))

	// Legacy: stored as a row in chatbot_flows with steps in
	// chatbot_flow_steps. canvas_layout column carries the position.
	legacyID := uuid.New()
	require.NoError(t, app.DB.Exec(`
		INSERT INTO chatbot_flows
		(id, organization_id, whats_app_account, name, is_enabled, canvas_layout, graph, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?::jsonb, NULL, NOW(), NOW())
	`, legacyID, org.ID, account.Name, "legacy", true, `{"node_positions":{"step_1":{"x":100,"y":200}}}`).Error)
	require.NoError(t, app.DB.Create(&models.ChatbotFlowStep{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		FlowID:      legacyID,
		StepName:    "step_1",
		StepOrder:   1,
		MessageType: "text",
		Message:     "Hello!",
	}).Error)

	// Already-v2: explicit Graph, no steps.
	preExisting := models.JSONB{
		"version":    2,
		"entry_node": "m1",
		"nodes": []any{
			map[string]any{"id": "m1", "type": "message", "config": map[string]any{"message": "hi"}},
		},
		"edges": []any{},
	}
	v2flow := &models.ChatbotFlow{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  org.ID,
		WhatsAppAccount: account.Name,
		Name:            "already-v2",
		IsEnabled:       true,
		Graph:           preExisting,
	}
	require.NoError(t, app.DB.Create(v2flow).Error)

	converted, err := BackfillChatbotFlowGraph(app.DB, app.Log)
	require.NoError(t, err)
	assert.Equal(t, 1, converted)

	var legacyAfter models.ChatbotFlow
	require.NoError(t, app.DB.First(&legacyAfter, legacyID).Error)
	require.NotNil(t, legacyAfter.Graph)
	assert.EqualValues(t, 2, legacyAfter.Graph["version"])
	assert.Equal(t, "__start__", legacyAfter.Graph["entry_node"])

	var v2After models.ChatbotFlow
	require.NoError(t, app.DB.First(&v2After, v2flow.ID).Error)
	require.NotNil(t, v2After.Graph)
	nodes, _ := v2After.Graph["nodes"].([]any)
	require.Len(t, nodes, 1)
	n0, _ := nodes[0].(map[string]any)
	assert.Equal(t, "m1", n0["id"])

	// Idempotent.
	converted, err = BackfillChatbotFlowGraph(app.DB, app.Log)
	require.NoError(t, err)
	assert.Zero(t, converted)
}

var _ = testutil.NopLogger // silence unused-import warning when read in isolation

// The flow list's "Steps" column counts authored graph nodes; the fixed
// start sentinel is not one of them.
func TestGraphStepCount(t *testing.T) {
	assert.Equal(t, 0, graphStepCount(nil))
	assert.Equal(t, 0, graphStepCount(models.JSONB{}))

	graph := models.JSONB{"nodes": []any{
		map[string]any{"id": "__start__", "type": "start"},
		map[string]any{"id": "m1", "type": "message"},
		map[string]any{"id": "b1", "type": "buttons"},
	}}
	assert.Equal(t, 2, graphStepCount(graph))
}
