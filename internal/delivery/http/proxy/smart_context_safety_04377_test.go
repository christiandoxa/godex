package proxy

import (
	"strings"
	"testing"
)

func TestProdex04377CriticalSignalsCountEveryLineAndLocation(t *testing.T) {
	line := "error[E0308]: mismatch src/lib.rs:1:2 @@ -1,2 +1,3 @@ test parses ... FAILED exit code 1 stack backtrace: warning: unused variable"
	got := smartContextCriticalSignalCounts(strings.Repeat(line+"\n", 2))
	want := smartContextSignalCounts{
		errors: 2, fileLocations: 2, diffHunks: 2, testFailures: 2,
		exitCodes: 2, stackMarkers: 2, rustDiagnostics: 4,
	}
	if got != want {
		t.Fatalf("critical signals = %#v, want %#v", got, want)
	}
}
