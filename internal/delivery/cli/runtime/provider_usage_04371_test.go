package runtime

import (
	"errors"
	"strings"
	"testing"
)

// Matches the canonical clap failure: unknown provider is a command-line
// usage error (exit 2), not an unavailable-runtime error (exit 1).
func TestProdex04371UnsupportedProviderReportsCanonicalUsageStatus(t *testing.T) {
	for _, value := range []string{"openai", "OPENAI", "unknown-provider"} {
		normalized, err := normalizeExternalProvider(value)
		if normalized != "" || err == nil {
			t.Fatalf("unsupported provider %q accepted", value)
		}
		var code interface{ ExitCode() int }
		if !errors.As(err, &code) || code.ExitCode() != 2 {
			t.Fatalf("unknown provider %q classified as runtime error: %v", value, err)
		}
		var argument interface{ CLIArgumentError() bool }
		if !errors.As(err, &argument) || !argument.CLIArgumentError() {
			t.Fatalf("unknown provider %q lost CLI reporting marker", value)
		}
		if !strings.Contains(err.Error(), "invalid --provider") {
			t.Fatalf("error message was lost: %v", err)
		}
	}
	for _, value := range []string{"deepseek", "gemini", "kiro", "copilot", "anthropic"} {
		selected, err := normalizeExternalProvider(value)
		if err != nil || selected != value {
			t.Fatalf("valid provider %q changed: value=%q err=%v", value, selected, err)
		}
	}
}
