package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Prodex 0.435.9 exposes --effort in the actual PingOpenaiArgs clap
// contract. Interactive model picking does not replace discoverability
// in non-interactive --help output.
func TestProdex04359PingHelpDocumentsEffortFlag(t *testing.T) {
	for _, args := range [][]string{
		{"ping", "openai", "--help"},
		{"help", "ping", "openai"},
	} {
		var output bytes.Buffer
		handled, err := printPublicCommandHelp(&output, args)
		if err != nil || !handled {
			t.Fatalf("help %v handled=%t error=%v", args, handled, err)
		}
		if !strings.Contains(output.String(), "--effort LEVEL") {
			t.Fatalf("0.435.9 --effort flag missing from help %v: %s", args, output.String())
		}
	}
}
