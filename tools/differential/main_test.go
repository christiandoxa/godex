package main

import "testing"

func TestNegativeControlDetectsClientStatusMismatch(t *testing.T) {
	if !negativeControl() {
		t.Fatal("comparison did not detect changed client status")
	}
}

func TestRedactRemovesSyntheticKey(t *testing.T) {
	if got := redact("Authorization: Bearer " + apiKey); got != "Authorization: Bearer <synthetic-key>" {
		t.Fatalf("redact = %q", got)
	}
}

func TestParityStatusRejectsProcessFailures(t *testing.T) {
	failed := productRun{ExitStatus: 1}
	status, _ := parityStatus(failed, failed)
	if status != "FAIL" {
		t.Fatalf("equal failures must fail, got %s", status)
	}
}

func TestParityStatusRejectsDifferentClientBody(t *testing.T) {
	left := productRun{Client: exchange{Status: 200, Body: "ok"}}
	right := productRun{Client: exchange{Status: 200, Body: "wrong"}}
	status, fields := parityStatus(left, right)
	if status != "FAIL" || !contains(fields, "client.body") {
		t.Fatalf("status=%s fields=%v", status, fields)
	}
}

func TestParityStatusAcceptsEquivalentSuccess(t *testing.T) {
	result := productRun{Client: exchange{Status: 200, Body: "ok"}}
	status, fields := parityStatus(result, result)
	if status != "PASS" || len(fields) != 0 {
		t.Fatalf("status=%s fields=%v", status, fields)
	}
}
