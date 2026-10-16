package runtime

import (
	"errors"
	"strings"
	"testing"
)

// Official Prodex 0.438.0 clap parser returns exit status 2 when --api-key
// occurs without its required --provider, before opening a provider process.
// A runtime failure after a syntactically valid invocation remains exit 1.
func TestProdex04380SuperAPIKeyWithoutProviderIsCLIUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--api-key", "synthetic-value", "--dry-run"},
		{"--dry-run", "--api-key", "synthetic-value"},
	} {
		_, err := parseSuperArguments(args)
		if err == nil || !strings.Contains(err.Error(), "--api-key requires --provider") {
			t.Fatalf("missing --provider incorrectly accepted for %v: %v", args, err)
		}
		var coded interface{ ExitCode() int }
		if !errors.As(err, &coded) || coded.ExitCode() != 2 {
			t.Fatalf("invalid CLI syntax must exit 2, got %v for %v", err, args)
		}
		var argument interface{ CLIArgumentError() bool }
		if !errors.As(err, &argument) || !argument.CLIArgumentError() {
			t.Fatal("CLI error lost diagnostic-print eligibility")
		}
	}
}
