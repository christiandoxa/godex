package main

import "testing"

func TestResidencyOracleRejectsMatchingHeaderCorruption(t *testing.T) {
	for _, residency := range []string{"", "us"} {
		fixture := startupProbeCase{wantState: true, residency: residency}
		good := startupProbeResult{snapshot: true, requests: []startupProbeRequest{{method: "GET", path: startupUsagePath, authValid: true, residency: residency}}}
		if !sameStartupProbeResult(good, good, fixture) {
			t.Fatal("rejected canonical residency probe")
		}
		bad := good
		bad.requests = append([]startupProbeRequest(nil), good.requests...)
		bad.requests[0].residency = "us"
		if residency == "us" {
			bad.requests[0].residency = ""
		}
		if sameStartupProbeResult(bad, bad, fixture) {
			t.Fatal("accepted matching missing or invented residency in both products")
		}
	}
}
