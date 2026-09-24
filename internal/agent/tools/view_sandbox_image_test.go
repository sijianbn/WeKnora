package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pngMagic is enough for the happy path: the tool sniffs the format from
// magic bytes, it never decodes pixels.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

func TestViewSandboxImageHappyPath(t *testing.T) {
	source := &fakeSandboxFileSource{
		stat: &sandbox.RemoteStatEntry{
			Path: "/workspace/input/gene-table.png",
			Type: sandbox.RemoteEntryFile,
			Size: int64(len(pngMagic)),
		},
		data: pngMagic,
	}

	result, err := NewViewSandboxImageTool(source).Execute(
		sandboxFileTestContext(),
		json.RawMessage(`{"path":"/workspace/input/gene-table.png"}`),
	)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Images, 1)
	assert.Contains(t, result.Images[0], "data:image/png;base64,")
	assert.Contains(t, result.Output, "/workspace/input/gene-table.png")
	assert.Equal(t, "image/png", result.Data["mime"])
	assert.Equal(t, len(pngMagic), result.Data["size"])
}

func TestViewSandboxImageRefusesOversizeBeforeRead(t *testing.T) {
	source := &fakeSandboxFileSource{
		stat: &sandbox.RemoteStatEntry{
			Path: "/workspace/input/huge.png",
			Type: sandbox.RemoteEntryFile,
			Size: maxViewSandboxImageBytes + 1,
		},
		data: pngMagic,
	}

	result, err := NewViewSandboxImageTool(source).Execute(
		sandboxFileTestContext(),
		json.RawMessage(`{"path":"/workspace/input/huge.png"}`),
	)

	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Contains(t, result.Error, "too large")
	assert.Equal(t, 1, source.statCalls)
	assert.Zero(t, source.readCalls, "oversized image must be refused at stat, bytes never fetched")
}

func TestViewSandboxImageRejectsDirectory(t *testing.T) {
	source := &fakeSandboxFileSource{
		stat: &sandbox.RemoteStatEntry{
			Path: "/workspace/input",
			Type: sandbox.RemoteEntryDir,
			Size: 0,
		},
	}

	result, err := NewViewSandboxImageTool(source).Execute(
		sandboxFileTestContext(),
		json.RawMessage(`{"path":"/workspace/input"}`),
	)

	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Contains(t, result.Error, "directory")
}

func TestViewSandboxImageRejectsNonImageBytes(t *testing.T) {
	// A renamed PDF must not ride the image path into the model's input:
	// the extension lies, the magic bytes do not.
	payload := []byte("%PDF-1.7 definitely not a png")
	source := &fakeSandboxFileSource{
		stat: &sandbox.RemoteStatEntry{
			Path: "/workspace/input/renamed.png",
			Type: sandbox.RemoteEntryFile,
			Size: int64(len(payload)),
		},
		data: payload,
	}

	result, err := NewViewSandboxImageTool(source).Execute(
		sandboxFileTestContext(),
		json.RawMessage(`{"path":"/workspace/input/renamed.png"}`),
	)

	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Empty(t, result.Images)
	assert.Contains(t, result.Error, "not a supported image")
}

func TestViewSandboxImageRejectsOutsideWorkspace(t *testing.T) {
	source := &fakeSandboxFileSource{
		stat: &sandbox.RemoteStatEntry{
			Path: "/etc/passwd",
			Type: sandbox.RemoteEntryFile,
			Size: 8,
		},
		data: pngMagic,
	}

	result, err := NewViewSandboxImageTool(source).Execute(
		sandboxFileTestContext(),
		json.RawMessage(`{"path":"/etc/passwd"}`),
	)

	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Zero(t, source.readCalls)
	assert.Contains(t, result.Error, "outside that scope")
}

func TestViewSandboxImageRequiresSession(t *testing.T) {
	result, err := NewViewSandboxImageTool(&fakeSandboxFileSource{}).Execute(
		context.Background(), // no ToolExecContext, no session ID
		json.RawMessage(`{"path":"/workspace/input/a.png"}`),
	)

	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Contains(t, result.Error, "no session ID")
}

func TestViewImageMIME(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		mime  string
	}{
		{"png", pngMagic, "image/png"},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, "image/jpeg"},
		{"gif89a", []byte("GIF89a\x00\x00"), "image/gif"},
		{"gif87a", []byte("GIF87a\x00\x00"), "image/gif"},
		{"webp", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WEBPVP8 ")...), "image/webp"},
		{"text", []byte("plain text"), ""},
		{"truncated", pngMagic[:4], ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.mime, viewImageMIME(tt.bytes))
		})
	}
}
