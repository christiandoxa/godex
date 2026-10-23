package main

import "testing"

func TestQuotaFailureOracleRejectsMatchingCorruption(t *testing.T) {
	for _, probe := range quotaFailureCases() {
		for _, mode := range []string{"once", "raw", "all"} {
			good := quotaFailureResult{code: 1, stderr: "quota unavailable", unchanged: true}
			if probe.config == "" {
				good.requests = []startupProbeRequest{{method: "GET", path: startupUsagePath, authValid: true}}
			}
			if mode == "all" {
				good.code, good.stdout, good.stderr = 0, "main error", ""
			}
			if err := verifyQuotaFailure(probe, mode, good); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*quotaFailureResult){
				func(r *quotaFailureResult) { r.code = 7 },
				func(r *quotaFailureResult) { r.unchanged = false },
				func(r *quotaFailureResult) { r.stdout, r.stderr = r.stderr, r.stdout },
				func(r *quotaFailureResult) { r.requests = append(r.requests, startupProbeRequest{}) },
				func(r *quotaFailureResult) { r.stderr += "synthetic-access" },
			} {
				bad := good
				mutate(&bad)
				if err := verifyQuotaFailure(probe, mode, bad); err == nil {
					t.Fatalf("accepted matching corruption for %s/%s", probe.name, mode)
				}
			}
		}
	}
}
