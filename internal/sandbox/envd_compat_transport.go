// Package sandbox: envd protocol compatibility for the E2B data plane.
//
// The sandbox-side daemon (envd) authenticates every data-plane call with HTTP
// Basic auth carrying the sandbox account name, and accepts file uploads only
// as multipart/form-data. github.com/matiasinsaurralde/go-e2b predates both:
// it sends the account in an X-User-ID header and POSTs file contents as a raw
// octet-stream body.
//
// E2B Cloud's own gateway is lenient enough to hide the difference, so the gap
// only surfaces against other implementations of the protocol — the very
// backends WeKnora wants to support without carrying one adapter per vendor.
// Rather than fork the SDK, this transport rewrites the data-plane details
// on the way out:
//
//   - it adds "Authorization: Basic base64(user:)" when the request carries no
//     credentials of its own;
//   - it re-wraps a non-multipart /files upload as multipart/form-data;
//   - and, for providers whose envd is not on the SDK's port, it rewrites the
//     authority's port label (see NewEnvdPortTransport below).
//
// Requests are recognised by path, not by host, so the shim works whether the
// data plane is reached directly or through a gateway (gateway_transport.go).
// Control-plane calls never use these paths and pass through untouched.
package sandbox

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
)

// envdFilesRoute is envd's upload/download endpoint.
const envdFilesRoute = "/files"

// envdUploadFormField is the multipart field name envd reads the payload from.
const envdUploadFormField = "file"

// NewEnvdCompatTransport wraps next so E2B data-plane requests satisfy the
// current envd contract. A blank user leaves authentication untouched.
func NewEnvdCompatTransport(next http.RoundTripper, user string) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &envdCompatTransport{next: next, user: strings.TrimSpace(user)}
}

type envdCompatTransport struct {
	next http.RoundTripper
	user string
}

func (t *envdCompatTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isEnvdDataPlaneRequest(req) {
		return t.next.RoundTrip(req)
	}
	rewritten := req.Clone(req.Context())
	if t.user != "" && rewritten.Header.Get("Authorization") == "" {
		user := t.user
		if req.URL.Path == envdFilesRoute || strings.HasPrefix(req.URL.Path, "/filesystem.Filesystem/") {
			user = remoteFileUser(req.Context())
		}
		rewritten.Header.Set("Authorization", basicAuthorizationFor(user))
	}
	if err := rewriteEnvdUpload(rewritten); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(rewritten)
}

func (t *envdCompatTransport) CloseIdleConnections() {
	if closer, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// isEnvdDataPlaneRequest reports whether req addresses envd rather than the
// control plane. envd serves /files plus the ConnectRPC services generated
// from its proto package.
func isEnvdDataPlaneRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	requestPath := req.URL.Path
	if requestPath == envdFilesRoute {
		return true
	}
	return strings.HasPrefix(requestPath, "/filesystem.Filesystem/") ||
		strings.HasPrefix(requestPath, "/process.Process/")
}

// basicAuthorizationFor builds envd's credential: the account name as the
// username with an empty password.
func basicAuthorizationFor(user string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"))
}

// rewriteEnvdUpload converts a raw-body upload into the multipart form envd
// expects. Uploads that already are multipart, and every non-upload request,
// are left alone.
func rewriteEnvdUpload(req *http.Request) error {
	if req.Method != http.MethodPost || req.URL.Path != envdFilesRoute {
		return nil
	}
	if strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/") {
		return nil
	}
	if req.Body == nil {
		return nil
	}
	payload, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return fmt.Errorf("sandbox: read envd upload body: %w", err)
	}

	filename := path.Base(strings.TrimSpace(req.URL.Query().Get("path")))
	if filename == "" || filename == "." || filename == "/" {
		filename = envdUploadFormField
	}

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile(envdUploadFormField, filename)
	if err != nil {
		return fmt.Errorf("sandbox: build envd upload form: %w", err)
	}
	if _, err := part.Write(payload); err != nil {
		return fmt.Errorf("sandbox: write envd upload form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("sandbox: close envd upload form: %w", err)
	}

	body := form.Bytes()
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = int64(len(body))
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return nil
}

// envdPortPrefix is the authority label the SDK pins onto every envd call:
// "49983-{sandboxID}.{domain}". Only this prefix is rewritten, never the ports
// of other data-plane services (desktop websockify dials 6080-), so a
// misconfigured port cannot silently redirect unrelated traffic.
const envdPortPrefix = "49983-"

// NewEnvdPortTransport rewrites the envd authority's port prefix from the
// SDK's hardcoded 49983 onto port.
//
// go-e2b exposes no override for the envd port, yet E2B-compatible providers
// are free to serve envd elsewhere — FC Agent Sandbox listens on 5000, and its
// gateway answers 49983- authorities with an HTTP-level 4xx that surfaces as
// "process rpc: HTTP status 400 Bad Request; command was not started". Like
// the two rewrites above, this is a data-plane detail fixed on the way out
// rather than an SDK fork.
//
// A port of 0 or the SDK's own 49983 returns next untouched, so backends that
// keep the default pay nothing. The rewrite applies only to envd requests —
// recognised by path, like isEnvdDataPlaneRequest — which also keeps a
// control-plane host that merely starts with "49983-" out of scope.
func NewEnvdPortTransport(next http.RoundTripper, port int) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	if port <= 0 || port == SDKEnvdPort {
		return next
	}
	return &envdPortTransport{next: next, port: port}
}

type envdPortTransport struct {
	next http.RoundTripper
	port int
}

func (t *envdPortTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isEnvdDataPlaneRequest(req) || !hasEnvdPortPrefix(req.URL.Host) {
		return t.next.RoundTrip(req)
	}
	// Clone rather than mutate: the caller owns req, and the SDK may retry it.
	rewritten := req.Clone(req.Context())
	url := *req.URL
	url.Host = fmt.Sprintf("%d-%s", t.port, strings.TrimPrefix(req.URL.Host, envdPortPrefix))
	rewritten.URL = &url
	return t.next.RoundTrip(rewritten)
}

func (t *envdPortTransport) CloseIdleConnections() {
	if closer, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// hasEnvdPortPrefix reports whether the authority carries the SDK's envd port
// label. The comparison is case-insensitive because DNS authorities are.
func hasEnvdPortPrefix(host string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(host)), envdPortPrefix)
}
