package codex

import (
	"path/filepath"
	"strings"
)

// PreviewRuntimeProxyArguments returns the exact Codex argument projection used
// by the runtime proxy without starting Codex or the proxy.
func PreviewRuntimeProxyArguments(provider string, arguments []string) ([]string, error) {
	const endpoint = "http://127.0.0.1:0"
	commandServer := codexCommandServerSubcommand(arguments)
	var (
		projected []string
		err       error
	)
	switch strings.TrimSpace(provider) {
	case "local":
		projected, err = localProxyArguments(endpoint, arguments)
	case "openai-compatible":
		projected, err = openAICompatibleProxyArguments(endpoint, arguments)
	default:
		projected, err = proxyArguments(endpoint, arguments)
	}
	if err != nil || commandServer {
		return projected, err
	}
	return codexTUIArguments(projected), nil
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
