package runtime

import (
	"errors"
	"math"
)

// Prodex 0.438.0 launch/recovery_exit_status_tests.rs extracts child exit
// and signal facts from the OS, then uses the compiled launch policy. Keep
// its signed-i32 Windows behavior and Unix SIGINT cancellation independent of
// error string text: a cancelled native turn must never be replayed.
func runtimeExitCancelledFacts04380(exitCode, signal, interruptSignal, controlCExit int64) bool {
	const minStatus, maxStatus = int64(math.MinInt32), int64(math.MaxInt32)
	if exitCode < minStatus || exitCode > maxStatus || signal < -1 || interruptSignal < -1 ||
		controlCExit < minStatus || controlCExit > maxStatus ||
		(controlCExit == -1 && exitCode < -1) {
		return false
	}
	return exitCode == 130 || (interruptSignal >= 0 && signal == interruptSignal) ||
		(controlCExit != -1 && exitCode == controlCExit)
}

func recoveryExitCancelled04360(err error) bool {
	type exitCoder interface{ ExitCode() int }
	var status exitCoder
	if err == nil || !errors.As(err, &status) {
		return false
	}
	signal, interrupt, controlC := recoveryPlatformSignalFacts04380(err)
	return runtimeExitCancelledFacts04380(recoveryExitCodeFact04380(status.ExitCode()), signal, interrupt, controlC)
}
