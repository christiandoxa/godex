//go:build linux

package main

import (
	"os"
	"testing"
)

// An arbitrary 200 or an open port cannot turn a surviving-but-stuck listener
// into PASS. Every invariant is independently mandatory on both products.
func TestProdex04380GatewayDescriptorPressureRequiresActualFDAndQueuedRecovery(t *testing.T) {
	good := gatewayPressureResult04380{
		requests: 3, maxFDs: gatewayPressureFdLimit04380,
		retained: true, queued: true, fresh: true, exited: true,
	}
	if !sameGatewayPressure04380(good, good) {
		t.Fatal("reference descriptor-pressure contract rejected")
	}
	bad := []gatewayPressureResult04380{
		{requests: 4, maxFDs: gatewayPressureFdLimit04380, retained: true, queued: true, fresh: true, exited: true},
		{requests: 3, maxFDs: gatewayPressureFdLimit04380 - 1, retained: true, queued: true, fresh: true, exited: true},
		{requests: 3, maxFDs: gatewayPressureFdLimit04380, retained: false, queued: true, fresh: true, exited: true},
		{requests: 3, maxFDs: gatewayPressureFdLimit04380, retained: true, queued: false, fresh: true, exited: true},
		{requests: 3, maxFDs: gatewayPressureFdLimit04380, retained: true, queued: true, fresh: false, exited: true},
		{requests: 3, maxFDs: gatewayPressureFdLimit04380, retained: true, queued: true, fresh: true, exited: false},
	}
	for index, mutated := range bad {
		if sameGatewayPressure04380(good, mutated) || sameGatewayPressure04380(mutated, good) {
			t.Fatalf("mutation %d evaded exact descriptor pressure oracle: %+v", index, mutated)
		}
	}
}

// Opt-in smoke uses the two exact runnable binaries but does not claim clean
// VCS provenance; the release harness separately pins source and SHA256.
func TestProdex04380GatewayDescriptorPressureRealExecutables(t *testing.T) {
	prodex := os.Getenv("GODEX_TEST_PRODEX_04380_BIN")
	candidate := os.Getenv("GODEX_TEST_GODEX_04380_BIN")
	if prodex == "" || candidate == "" {
		t.Skip("requires explicit pinned binary paths")
	}
	cases, err := checkGatewayFDPressure(t.TempDir(), cliOptions{prodex: prodex, godex: candidate})
	if err != nil || len(cases) != 1 || cases[0].Requests != 3 {
		t.Fatalf("real Prodex/Godex FD saturation divergence: cases=%+v err=%v", cases, err)
	}
}
