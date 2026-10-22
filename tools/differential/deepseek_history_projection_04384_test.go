package main

import "testing"

func TestDeepSeekHistoryProjectionRejectsMatchingCorruption(t *testing.T) {
	if !deepSeekHistoryNegativeControl04384() {
		t.Fatal("history seed/request/state corruption escaped independent checks")
	}
}
