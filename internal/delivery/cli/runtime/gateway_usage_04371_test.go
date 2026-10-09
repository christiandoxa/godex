package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// Malformed gateway CLI syntax is a canonical argument/usage error (exit 2).
// A valid request with unconfigured runtime support is still a runtime
// failure and must not be silently reclassified as usage.
func TestProdex04371GatewayMalformedArgumentsUseExitTwo(t *testing.T) {
	fixtures := [][]string{
		{"--no-such-option"},
		{"--provider"},
		{"--listen"},
		{"--base-url"},
		{"--api-key"},
		{"positional"},
		{"--presidio", "--no-presidio"},
		{"--listen", "127.0.0.1:0", "--no-such-option"},
		{"--provider", "openai"},
		{"--provider", "nonsense"},
	}
	for _, arguments := range fixtures {
		t.Run(strings.Join(arguments, "_"), func(t *testing.T) {
			err := Gateway(context.Background(), nil, nil, io.Discard, arguments)
			if err == nil {
				t.Fatal("malformed gateway arguments accepted")
			}
			var coder interface{ ExitCode() int }
			if !errors.As(err, &coder) || coder.ExitCode() != 2 {
				t.Fatalf("gateway parser returned runtime status instead of 2: %v", err)
			}
			var printable interface{ CLIArgumentError() bool }
			if !errors.As(err, &printable) || !printable.CLIArgumentError() {
				t.Fatalf("malformed gateway error lost stderr marker: %v", err)
			}
		})
	}
	err := Gateway(context.Background(), nil, nil, io.Discard, []string{"--listen", "127.0.0.1:0"})
	if err == nil {
		t.Fatal("runtime without runner incorrectly started")
	}
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		t.Fatalf("runtime misconfiguration became parser exit: %v", err)
	}
}
