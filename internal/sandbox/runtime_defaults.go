package sandbox

import (
	"net/url"
	"strings"
)

// Runtime tuning has safe built-in defaults. Unlike endpoints, credentials,
// and templates, these values do not identify an external backend.
func applyCubeRuntimeDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.CubeSandboxTTL <= 0 {
		cfg.CubeSandboxTTL = DefaultCubeSandboxTTL
	}
	if cfg.CubeHTTPTimeout <= 0 {
		cfg.CubeHTTPTimeout = DefaultCubeHTTPTimeout
	}
}

func applyDockerRuntimeDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	// The image is deliberately not defaulted here: it is this backend's
	// template, and a config that fails to name one must be reported as
	// incomplete rather than silently pointed at whatever image the release
	// happens to ship.
	if cfg.DockerHost == "" {
		cfg.DockerHost = DetectLocalDockerHost()
	}
	if cfg.DockerCPULimit <= 0 {
		cfg.DockerCPULimit = DefaultDockerCPULimit
	}
	if cfg.DockerMemoryBytes <= 0 {
		cfg.DockerMemoryBytes = DefaultDockerMemoryLimit
	}
	if cfg.DockerPidsLimit <= 0 {
		cfg.DockerPidsLimit = DefaultDockerPidsLimit
	}
	if cfg.DockerIdleTTL <= 0 {
		cfg.DockerIdleTTL = DefaultDockerIdleTTL
	}
	if cfg.DockerHTTPTimeout <= 0 {
		cfg.DockerHTTPTimeout = DefaultDockerHTTPTimeout
	}
}

func applyE2BRuntimeDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.E2BSandboxTTL <= 0 {
		cfg.E2BSandboxTTL = DefaultE2BSandboxTTL
	}
	if cfg.E2BHTTPTimeout <= 0 {
		cfg.E2BHTTPTimeout = DefaultE2BHTTPTimeout
	}
	// The envd port is derived from the API endpoint rather than defaulted
	// blindly: rewriting it is only correct for providers that actually serve
	// envd elsewhere, and for everyone else the SDK's hardcoded 3000 must
	// stand. An explicit envd_port always wins.
	if cfg.E2BEnvdPort <= 0 && isFCAliyunAgentSandboxAPI(cfg.E2BAPIURL) {
		cfg.E2BEnvdPort = FCAliyunEnvdPort
	}
}

// isFCAliyunAgentSandboxAPI reports whether apiURL addresses an FC Agent
// Sandbox control plane, recognised by its regional *.fc.aliyuncs.com host
// (e.g. api.cn-hangzhou.e2b.fc.aliyuncs.com).
func isFCAliyunAgentSandboxAPI(apiURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host != "" && strings.HasSuffix(strings.ToLower(host), ".fc.aliyuncs.com")
}
