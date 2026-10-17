package handlers

import (
	"testing"

	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text with {{variables}} only has a value during the call, so saving the flow
// must not synthesize it (it would speak the literal braces). The piper binary
// here doesn't exist: any attempt to synthesize would fail the save.
func TestGenerateIVRAudio_SkipsTextWithVariables(t *testing.T) {
	a := &App{TTS: &tts.PiperTTS{BinaryPath: "/nonexistent/piper", ModelPath: "x.onnx", AudioDir: t.TempDir()}}

	menu := models.JSONB{
		"version":    2,
		"entry_node": "m",
		"nodes": []any{
			map[string]any{"id": "m", "type": "menu", "config": map[string]any{
				"greeting_text": "{{slots_prompt}}",
				"audio_file":    "",
			}},
		},
	}

	require.NoError(t, a.generateIVRAudio(menu))
	nodes := menu["nodes"].([]any)
	cfg := nodes[0].(map[string]any)["config"].(map[string]any)
	assert.Equal(t, "", cfg["audio_file"], "no audio is prepared for live text")

	// Static text still goes through TTS at save time.
	menu["nodes"] = []any{
		map[string]any{"id": "g", "type": "greeting", "config": map[string]any{"greeting_text": "Welcome"}},
	}
	assert.Error(t, a.generateIVRAudio(menu))
}
