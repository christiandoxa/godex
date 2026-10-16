//go:build windows

package runtime

import "math"

const windowsCtrlCExit04380 = int64(-1073741510) // signed NTSTATUS 0xC000013A

// Windows Go os/exec can expose an exit status through a native signed i32 or
// as its unsigned uint32 representation on a 64-bit host. Both are the same
// NTSTATUS; an unrelated access violation must never suppress recovery.
func recoveryExitCodeFact04380(code int) int64 {
	if code > math.MaxInt32 && uint64(code) <= math.MaxUint32 {
		return int64(int32(uint32(code)))
	}
	return int64(code)
}
func recoveryPlatformSignalFacts04380(error) (int64, int64, int64) {
	return -1, -1, windowsCtrlCExit04380
}
