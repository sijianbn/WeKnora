package agent

import (
	"context"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// view_sandbox_image returns pixels in ToolResult.Images; delivery rides the
// generic appendToolImages path shared with local_browser and MCP images.
func viewSandboxImageStep() types.AgentStep {
	return types.AgentStep{
		Thought: "checking the image attachment",
		ToolCalls: []types.ToolCall{{
			ID:   "call-1",
			Name: agenttools.ToolViewSandboxImage,
			Args: map[string]interface{}{"path": "/workspace/input/gene-table.png"},
			Result: &types.ToolResult{
				Success: true,
				Output:  "Image loaded: /workspace/input/gene-table.png (image/png, 8 bytes).",
				Images:  []string{"data:image/png;base64,AAAA"},
				Data:    map[string]interface{}{"path": "/workspace/input/gene-table.png"},
			},
		}},
	}
}

// A vision chat model gets the pixels re-emitted as a user-role image
// message — the one image path every OpenAI-compatible gateway renders.
func TestViewSandboxImagePixelsReachVisionModel(t *testing.T) {
	engine := &AgentEngine{config: &types.AgentConfig{ChatModelSupportsVision: true}}
	step := viewSandboxImageStep()

	messages := engine.appendToolResults(nil, step)
	messages = engine.appendToolImages(t.Context(), messages, step)

	require.Len(t, messages, 3, "assistant + tool result + injected image message")
	injected := messages[2]
	assert.Equal(t, "user", injected.Role)
	assert.Contains(t, injected.Content, agenttools.ToolViewSandboxImage)
	require.Len(t, injected.Images, 1)
	assert.Equal(t, "data:image/png;base64,AAAA", injected.Images[0])
	assert.Empty(t, messages[1].Images, "the tool-role message stays text-only")
	assert.Equal(t, "Image loaded: /workspace/input/gene-table.png (image/png, 8 bytes).",
		step.ToolCalls[0].Result.Output, "vision path must not alter the persisted output")
}

// A non-vision model with a VLM describer gets the description appended to
// the live tool message; the persisted Output keeps the compact text only.
func TestViewSandboxImageNonVisionGetsVLMDescription(t *testing.T) {
	engine := &AgentEngine{imageDescriber: func(_ context.Context, _ []byte, _ string) (string, error) {
		return "基因表：ZBP1 AZU1 FCN1 HP TECR", nil
	}}
	step := viewSandboxImageStep()

	messages := engine.appendToolResults(nil, step)
	messages = engine.appendToolImages(t.Context(), messages, step)

	require.Len(t, messages, 2, "no injected image message without vision support")
	assert.Contains(t, messages[1].Content, "ZBP1", "the live tool message carries the description")
	assert.Equal(t, "Image loaded: /workspace/input/gene-table.png (image/png, 8 bytes).",
		step.ToolCalls[0].Result.Output, "fallback must not alter the persisted output")
}

// Neither vision nor VLM: the transcript says plainly that the image was
// not seen, so the model never claims it inspected pixels it could not.
func TestViewSandboxImageNoDeliverySaysUnseen(t *testing.T) {
	engine := &AgentEngine{}
	step := viewSandboxImageStep()

	messages := engine.appendToolResults(nil, step)
	messages = engine.appendToolImages(t.Context(), messages, step)

	require.Len(t, messages, 2)
	assert.Contains(t, messages[1].Content, "cannot view")
}
