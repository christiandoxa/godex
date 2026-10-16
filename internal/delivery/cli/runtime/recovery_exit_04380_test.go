package runtime

import (
	"errors"
	"testing"
)

type fixtureChildExit04380 int

func (code fixtureChildExit04380) Error() string { return "synthetic child exit" }
func (code fixtureChildExit04380) ExitCode() int { return int(code) }

// Official tagged Prodex 0.438.0 launch/recovery_exit_status_tests.rs.
// Control-C is signed NTSTATUS, not a positive integer or a generic error.
func TestProdex04380RuntimeExitCancellationSignedPolicy(t *testing.T) {
	controlC := int64(int32(-1073741510))
	for _, tc := range []struct {
		name                          string
		exit, signal, interrupt, ctrl int64
		want                          bool
	}{
		{"successful exit", 0, -1, -1, controlC, false},
		{"ordinary failure", 1, -1, -1, controlC, false},
		{"codex exit 130", 130, -1, -1, controlC, true},
		{"windows signed ctrl-c", controlC, -1, -1, controlC, true},
		{"windows access violation", -1073741819, -1, -1, controlC, false},
		{"windows other negative", -2, -1, -1, controlC, false},
		{"unknown exit", -1, -1, -1, controlC, false},
		{"minimum signed exit", -2147483648, -1, -1, controlC, false},
		{"maximum signed exit", 2147483647, -1, -1, controlC, false},
		{"unix SIGINT", -1, 2, 2, -1, true},
		{"unix SIGTERM", -1, 15, 2, -1, false},
		{"no platform facts", -1, -1, -1, -1, false},
		{"missing interrupt", -1, 2, -1, -1, false},
		{"negative status without Windows", -1073741510, -1, -1, -1, false},
		{"windows ctrl is not any failure", 1, -1, -1, controlC, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimeExitCancelledFacts04380(tc.exit, tc.signal, tc.interrupt, tc.ctrl); got != tc.want {
				t.Fatalf("exit=%d signal=%d interrupt=%d ctrl=%d got=%t want=%t", tc.exit, tc.signal, tc.interrupt, tc.ctrl, got, tc.want)
			}
		})
	}
}

func TestProdex04380RuntimeExitCancellationErrorClassification(t *testing.T) {
	if !recoveryExitCancelled04360(fixtureChildExit04380(130)) {
		t.Fatal("exit 130 must suppress recovery")
	}
	for _, code := range []int{0, 1, -1, -2, -1073741819, 2147483647} {
		if recoveryExitCancelled04360(fixtureChildExit04380(code)) {
			t.Fatalf("unrelated process status %d suppressed recovery", code)
		}
	}
	if recoveryExitCancelled04360(errors.New("ctrl-c string alone")) {
		t.Fatal("untyped string matching must not suppress recovery")
	}
}
