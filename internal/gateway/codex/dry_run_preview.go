package codex

import (
	"path/filepath"
	"strings"
)

// PreviewRuntimeProxyArguments returns the exact Codex argument projection used
// by the runtime proxy without starting Codex or the proxy.
func PreviewRuntimeProxyArguments(provider string, arguments []string) ([]string, error) {
	const endpoint = "http://127.0.0.1:0"
	switch strings.TrimSpace(provider) {
	case "local":
		return localProxyArguments(endpoint, arguments)
	case "openai-compatible":
		return openAICompatibleProxyArguments(endpoint, arguments)
	default:
		return proxyArguments(endpoint, arguments)
	}
}

// RuntimeDryRunBinaryLabel reports the configured child binary without probing
// or executing it. Dry-run output intentionally exposes only the basename.
func (process *CodexProcess) RuntimeDryRunBinaryLabel() string {
	if process == nil {
		return "codex"
	}
	candidate := strings.TrimSpace(process.binary)
	if candidate == "" {
		candidate = "codex"
	}
	if base := filepath.Base(candidate); base != "" && base != "." {
		return base
	}
	return "codex"
}
