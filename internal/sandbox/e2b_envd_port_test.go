package sandbox

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// The SDK pins every envd authority at "3000-{sandboxID}.{domain}" with no
// override, so a provider serving envd elsewhere needs the label rewritten on
// the way out. FC Agent Sandbox is that provider today (5000).
func TestEnvdPortTransportRewritesEnvdAuthority(t *testing.T) {
	for _, path := range []string{
		"/process.Process/Start",
		"/filesystem.Filesystem/Stat",
		"/files",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := &recordingRoundTripper{}
			transport := NewEnvdPortTransport(recorder, FCAliyunEnvdPort)

			request := httptest.NewRequest(http.MethodPost,
				"https://49983-sbx-test123.cn-hangzhou.e2b.fc.aliyuncs.com"+path,
				strings.NewReader("{}"))
			_, err := transport.RoundTrip(request)
			require.NoError(t, err)

			require.Equal(t,
				"5000-sbx-test123.cn-hangzhou.e2b.fc.aliyuncs.com",
				recorder.request.URL.Host,
			)
		})
	}
}

// Only the envd port label moves: websockify's 6080- authority, requests that
// are not envd calls at all, and control-plane traffic all keep their host.
func TestEnvdPortTransportLeavesOtherTrafficAlone(t *testing.T) {
	cases := []struct {
		name string
		url  string
		host string
	}{
		{
			"desktop port",
			"https://6080-sbx-test123.example.com/files",
			"6080-sbx-test123.example.com",
		},
		{
			"non-envd path on envd port",
			"https://49983-sbx-test123.example.com/healthz",
			"49983-sbx-test123.example.com",
		},
		{
			"control plane",
			"https://api.cn-hangzhou.e2b.fc.aliyuncs.com/templates",
			"api.cn-hangzhou.e2b.fc.aliyuncs.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &recordingRoundTripper{}
			transport := NewEnvdPortTransport(recorder, FCAliyunEnvdPort)

			request := httptest.NewRequest(http.MethodGet, tc.url, nil)
			_, err := transport.RoundTrip(request)
			require.NoError(t, err)

			require.Equal(t, tc.host, recorder.request.URL.Host)
		})
	}
}

// A disabled rewrite must be free: the constructor hands back the wrapped
// transport untouched, not a pass-through that clones every request.
func TestEnvdPortTransportDisabledWithoutOverride(t *testing.T) {
	recorder := &recordingRoundTripper{}
	require.Same(t, recorder, NewEnvdPortTransport(recorder, 0))
	require.Same(t, recorder, NewEnvdPortTransport(recorder, SDKEnvdPort))
}

// The E2B client chains compat over port rewrite over inbound tokens. The
// chain must deliver all three at once: rewritten authority, attached inbound
// token, and the Basic auth envd expects.
func TestEnvdClientTransportChainRewritesPortKeepsAuth(t *testing.T) {
	recorder := &recordingRoundTripper{}
	tokens := NewInboundTokenRegistry()
	tokens.Put("sbx-test123", "token-1")
	transport := NewEnvdCompatTransport(
		NewEnvdPortTransport(attachInboundTokenTransport(recorder, tokens), FCAliyunEnvdPort),
		DefaultSandboxExecUser,
	)

	request := httptest.NewRequest(http.MethodPost,
		"https://49983-sbx-test123.example.com/process.Process/Start",
		strings.NewReader("{}"))
	_, err := transport.RoundTrip(request)
	require.NoError(t, err)

	require.Equal(t, "5000-sbx-test123.example.com", recorder.request.URL.Host)
	require.Equal(t, "token-1", recorder.request.Header.Get(InboundTokenHeader))
	require.Equal(t,
		"Basic "+base64.StdEncoding.EncodeToString([]byte(DefaultSandboxExecUser+":")),
		recorder.request.Header.Get("Authorization"),
	)
}

// FC Agent Sandbox reports its credential as envdAccessToken, which go-e2b
// sends only as X-Access-Token while TrafficAccessToken — what the registry is
// fed from — stays empty. The inbound-token layer must then fall back to the
// X-Access-Token the SDK already set, or the provider's ingress denies every
// data-plane call.
func TestInboundTokenFallsBackToSDKAccessToken(t *testing.T) {
	recorder := &recordingRoundTripper{}
	transport := attachInboundTokenTransport(recorder, NewInboundTokenRegistry())

	request := httptest.NewRequest(http.MethodPost,
		"https://49983-sbx-fc.example.com/process.Process/Start",
		strings.NewReader("{}"))
	request.Header.Set("X-Access-Token", "envd-token-fc")
	_, err := transport.RoundTrip(request)
	require.NoError(t, err)

	require.Equal(t, "envd-token-fc", recorder.request.Header.Get(InboundTokenHeader))
}

// A registry hit is authoritative: E2B Cloud's traffic token is distinct from
// the envd access token, and the fallback must never mask it.
func TestInboundTokenRegistryWinsOverSDKAccessToken(t *testing.T) {
	recorder := &recordingRoundTripper{}
	tokens := NewInboundTokenRegistry()
	tokens.Put("sbx-e2b", "traffic-token")
	transport := attachInboundTokenTransport(recorder, tokens)

	request := httptest.NewRequest(http.MethodPost,
		"https://49983-sbx-e2b.example.com/process.Process/Start",
		strings.NewReader("{}"))
	request.Header.Set("X-Access-Token", "envd-token-e2b")
	_, err := transport.RoundTrip(request)
	require.NoError(t, err)

	require.Equal(t, "traffic-token", recorder.request.Header.Get(InboundTokenHeader))
}

func TestApplyE2BRuntimeDefaultsDerivesFCEnvdPort(t *testing.T) {
	cases := []struct {
		name   string
		apiURL string
		stored int
		want   int
	}{
		{
			"fc aliyun api derives 5000",
			"https://api.cn-hangzhou.e2b.fc.aliyuncs.com",
			0, FCAliyunEnvdPort,
		},
		{
			"fc aliyun other region derives 5000",
			"https://api.cn-beijing.e2b.fc.aliyuncs.com",
			0, FCAliyunEnvdPort,
		},
		{
			"e2b cloud keeps sdk default",
			"https://api.e2b.app",
			0, 0,
		},
		{
			"self-hosted keeps sdk default",
			"https://e2b.internal.example.com",
			0, 0,
		},
		{
			"empty api url keeps sdk default",
			"",
			0, 0,
		},
		{
			"explicit port wins over fc derivation",
			"https://api.cn-hangzhou.e2b.fc.aliyuncs.com",
			49983, 49983,
		},
		{
			"explicit sdk port stays",
			"https://api.cn-hangzhou.e2b.fc.aliyuncs.com",
			SDKEnvdPort, SDKEnvdPort,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Type: SandboxTypeE2B, E2BAPIURL: tc.apiURL, E2BEnvdPort: tc.stored}
			applyE2BRuntimeDefaults(cfg)
			require.Equal(t, tc.want, cfg.E2BEnvdPort)
		})
	}
}

func TestResolveEffectiveConfigEnvdPort(t *testing.T) {
	tenantCfg := &types.TenantSandboxConfig{
		SandboxType: "e2b",
		E2B: &types.E2BSandboxConfig{
			APIURL:     "https://api.cn-hangzhou.e2b.fc.aliyuncs.com",
			APIKey:     "key",
			TemplateID: "tpl",
		},
	}

	cfg, err := ResolveEffectiveConfig(tenantCfg, DefaultConfig())
	require.NoError(t, err)
	require.Equal(t, FCAliyunEnvdPort, cfg.E2BEnvdPort)

	tenantCfg.E2B.EnvdPort = 49983
	cfg, err = ResolveEffectiveConfig(tenantCfg, DefaultConfig())
	require.NoError(t, err)
	require.Equal(t, 49983, cfg.E2BEnvdPort)

	tenantCfg.E2B.EnvdPort = 0
	tenantCfg.E2B.APIURL = "https://api.e2b.app"
	cfg, err = ResolveEffectiveConfig(tenantCfg, DefaultConfig())
	require.NoError(t, err)
	require.Equal(t, 0, cfg.E2BEnvdPort)
}

// The envd port decides whether an existing sandbox is still reachable, so it
// belongs in the identity next to the domain: an edit that changes it must
// cordon rather than strand live sessions on the old port.
func TestIdentityOfIncludesEnvdPort(t *testing.T) {
	base := &types.TenantSandboxConfig{
		SandboxType: "e2b",
		E2B: &types.E2BSandboxConfig{
			APIURL:   "https://api.example.com",
			APIKey:   "key",
			EnvdPort: 5000,
		},
	}
	samePort := &types.TenantSandboxConfig{
		SandboxType: "e2b",
		E2B: &types.E2BSandboxConfig{
			APIURL:     "https://api.example.com",
			APIKey:     "key",
			EnvdPort:   5000,
			HTTPTimeoutSec: 99, // runtime tuning, not identity
		},
	}
	otherPort := &types.TenantSandboxConfig{
		SandboxType: "e2b",
		E2B: &types.E2BSandboxConfig{
			APIURL:   "https://api.example.com",
			APIKey:   "key",
			EnvdPort: 5001,
		},
	}

	require.Equal(t, IdentityOf(samePort), IdentityOf(base))
	require.NotEqual(t, IdentityOf(otherPort), IdentityOf(samePort))
}
