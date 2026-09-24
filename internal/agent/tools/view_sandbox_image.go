// Package tools — view_sandbox_image.
//
// Loads one image from the session sandbox into the model's multimodal
// input. read_file and shell_exec deliberately suppress binary bytes, so
// before this tool an image extracted into /workspace (a .msg attachment,
// a chart a script just rendered) had no path into the conversation at all.
//
// The tool itself only reads and reports; delivery happens in the engine's
// generic tool-image path (AgentEngine.appendToolImages, shared with
// local_browser and MCP images), because only the engine knows whether the
// chat model can accept pixels:
//   - vision chat model → the data URIs are re-emitted as a user-role image
//     message (the one image path every OpenAI-compatible gateway renders;
//     tool-role images are silently dropped by several providers).
//   - non-vision model with a VLM describer → a text description is appended
//     to the tool result instead.
//   - neither → an explicit "image unseen" note. The tool result never
//     claims the model saw something it did not.
//
// Guardrails mirror the chat upload path (internal/handler/session/
// image_upload.go): 10MB per image and a png/jpeg/gif/webp whitelist,
// enforced by content sniffing — file extensions and stat metadata are not
// trusted.
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// maxViewSandboxImageBytes caps one view_sandbox_image read. Mirrors the chat upload limit
// so a sandbox file can never exceed what the same bytes would cost through
// the chat attachment path.
const maxViewSandboxImageBytes = 10 << 20 // 10MB, aligned with image_upload.go

var viewSandboxImageTool = BaseTool{
	name: ToolViewSandboxImage,
	description: "View one image file from the session sandbox. The pixels are injected into the conversation so you can read text, tables and figures inside sandbox files (email attachment extracts, rendered charts, screenshots). " +
		"Use this instead of read_file for images — read_file suppresses binary content. One image per call, up to 10MB, png/jpeg/gif/webp.",
	schema: utils.GenerateSchema[ViewSandboxImageInput](),
}

// ViewSandboxImageInput defines the input parameters for view_sandbox_image.
type ViewSandboxImageInput struct {
	// Path is an absolute path inside the session sandbox, under /workspace
	// (the same scope read_file/list_sandbox_files accept).
	Path string `json:"path" jsonschema:"Absolute sandbox path of the image, under /workspace"`
}

// ViewSandboxImageTool exposes SandboxFileSource.ReadSessionFile to the agent as a
// multimodal image input primitive.
type ViewSandboxImageTool struct {
	BaseTool
	sessionBound
	source SandboxFileSource
}

// NewViewSandboxImageTool constructs the tool. `source` MUST NOT be nil: callers
// feature-gate registration on the sandbox backend advertising per-session
// file access (same condition as read_file).
func NewViewSandboxImageTool(source SandboxFileSource) *ViewSandboxImageTool {
	return &ViewSandboxImageTool{
		BaseTool: viewSandboxImageTool,
		source:   source,
	}
}

// BindSession records the session so Parameters() advertises that session's
// workspace instead of the remote /workspace copy.
func (t *ViewSandboxImageTool) BindSession(id string) {
	if t == nil {
		return
	}
	t.sessionBound.BindSession(id)
}

// Parameters rewrites workspace paths in the schema to match the session layout.
func (t *ViewSandboxImageTool) Parameters() json.RawMessage {
	if t == nil {
		return nil
	}
	return schemaForLayout(t.schema, t.boundLayout())
}

func (t *ViewSandboxImageTool) boundLayout() sandbox.WorkspaceLayout {
	if t == nil || t.source == nil {
		return sandbox.RemoteWorkspaceLayout()
	}
	return t.describeLayout(t.source)
}

// Execute reads one image from the current session's sandbox and returns it
// as a base64 data URI in ToolResult.Images for the engine to deliver.
func (t *ViewSandboxImageTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][ViewImage] Execute started")

	var input ViewSandboxImageInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse args: %v", err),
		}, nil
	}

	if t.source == nil {
		return &types.ToolResult{
			Success: false,
			Error:   "sandbox file access is not available in this deployment",
		}, nil
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "path is required; pass the sandbox path of the image (under /workspace)",
		}, nil
	}

	sessionID := resolveSessionID(ctx)
	if sessionID == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "no session ID in context; view_sandbox_image must run inside an agent turn",
		}, nil
	}

	// The source binds reads to the current session's sandbox. Unlike
	// read_file there is no "/" fallback for misses: an image viewer has no
	// reason to reach outside the session's read roots.
	layout, layoutErr := executeWorkspaceLayout(ctx, sessionID, t.source)
	if layoutErr != nil {
		return layoutErr, nil
	}
	clean := resolveIn(layout, input.Path)
	if _, ok := inspectableRootIn(layout, clean); !ok {
		return &types.ToolResult{
			Success: false,
			Error:   inspectScopeErrorIn(layout, clean),
		}, nil
	}

	// Stat first so an oversized or non-file path is rejected before its
	// bytes are ever pulled out of the sandbox.
	ctx = sandbox.WithSessionFileOperation(ctx)
	stat, err := t.source.StatSessionFile(ctx, sessionID, clean)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to stat %s: %v", clean, err),
		}, nil
	}
	if stat == nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("file not found: %s", clean),
		}, nil
	}
	if stat.Type == sandbox.RemoteEntryDir {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("%s is a directory, not an image", clean),
		}, nil
	}
	if stat.Type != sandbox.RemoteEntryFile {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("%s is not a regular file", clean),
		}, nil
	}
	if stat.Size > maxViewSandboxImageBytes {
		return &types.ToolResult{
			Success: false,
			Error: fmt.Sprintf("image %s is too large (%d bytes, max %d)",
				clean, stat.Size, maxViewSandboxImageBytes),
		}, nil
	}

	imgBytes, err := t.source.ReadSessionFile(ctx, sessionID, clean)
	if err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read %s: %v", clean, err),
		}, nil
	}
	if len(imgBytes) > maxViewSandboxImageBytes {
		return &types.ToolResult{
			Success: false,
			Error: fmt.Sprintf("image %s is too large (%d bytes, max %d)",
				clean, len(imgBytes), maxViewSandboxImageBytes),
		}, nil
	}

	mimeType := viewImageMIME(imgBytes)
	if mimeType == "" {
		return &types.ToolResult{
			Success: false,
			Error: fmt.Sprintf("%s is not a supported image (content sniffed as none of png/jpeg/gif/webp); "+
				"convert it first or extract the text another way", clean),
		}, nil
	}

	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)

	logger.Infof(ctx, "[Tool][ViewImage] session=%s path=%s mime=%s size=%d",
		sessionID, clean, mimeType, len(imgBytes))

	return &types.ToolResult{
		Success: true,
		Output: fmt.Sprintf("Image loaded: %s (%s, %d bytes). The pixels accompany this result in the "+
			"live turn; history replays keep only this text — call view_sandbox_image again to re-view. "+
			"Read what is actually there; do not guess at contents you cannot make out.",
			clean, mimeType, len(imgBytes)),
		Images: []string{dataURI},
		Data: map[string]interface{}{
			"session_id": sessionID,
			"path":       clean,
			"mime":       mimeType,
			"size":       len(imgBytes),
		},
	}, nil
}

// Cleanup releases any resources.
func (t *ViewSandboxImageTool) Cleanup(ctx context.Context) error {
	return nil
}

// allowedViewImageMIMEs is the image whitelist, aligned with the chat upload
// path (image_upload.go's mimeToExt set). The stdlib detector also names
// bmp/ico/text and other formats this tool deliberately does not accept.
var allowedViewImageMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// viewImageMIME returns the image MIME type for b when it is on the
// whitelist, "" otherwise. Extensions and Content-Type headers are
// untrusted: a renamed .exe must not ride the image path into the model's
// input. Uses the stdlib sniffer, like the rest of the codebase.
func viewImageMIME(b []byte) string {
	if m := http.DetectContentType(b); allowedViewImageMIMEs[m] {
		return m
	}
	return ""
}
