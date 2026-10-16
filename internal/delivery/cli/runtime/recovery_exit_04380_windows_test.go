//go:build windows

package runtime

import "testing"

func TestProdex04380WindowsNativeExitCodeNormalizesSignedNTSTATUS(t *testing.T) {
	const signed = -1073741510
	if !recoveryExitCancelled04360(fixtureChildExit04380(signed)) {
		t.Fatal("signed Windows Ctrl-C must stop native-session recovery")
	}
	if !recoveryExitCancelled04360(fixtureChildExit04380(int(uint32(0xC000013A)))) {
		t.Fatal("unsigned Windows representation must normalize to Ctrl-C")
	}
	if recoveryExitCancelled04360(fixtureChildExit04380(-1073741819)) {
		t.Fatal("Windows access violation is not user interruption")
	}
}
