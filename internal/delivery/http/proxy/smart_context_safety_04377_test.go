package proxy

import (
	"strings"
	"testing"
)

func TestProdex04377CriticalSignalsCountEveryLineAndLocation(t *testing.T) {
	text := strings.Join([]string{
		"error[E0308]: mismatch src/lib.rs:1:2",
		"error[E0425]: missing src/main.rs:3:4",
		"@@ -1,2 +1,3 @@",
		"@@ -4,5 +4,6 @@",
		"test parses ... FAILED",
		"another test ... FAILED",
		"exit code 1",
		"exit status 2",
		"stack backtrace:",
		"Backtrace:",
		"warning: unused variable",
		"warning: unused import",
	}, "\n")
	got := smartContextCriticalSignalCounts(text)
	want := smartContextSignalCounts{
		errors: 2, fileLocations: 2, diffHunks: 2, testFailures: 2,
		exitCodes: 2, stackMarkers: 2, rustDiagnostics: 4,
	}
	if got != want {
		t.Fatalf("critical signals = %#v, want %#v", got, want)
	}
}
