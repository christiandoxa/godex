package main

import (
	"strings"
	"testing"
)

func TestProdex04383TransportOracleRejectsMaterialMutations(t *testing.T) {
	clean := strings.ReplaceAll(transportHistory04383, `"x-codex-turn-state":"turn-a",`, "")
	base := []transportAttempt04383{
		{owner: "account-a", turn: "turn-a", body: transportHistory04383, ready: true},
		{owner: "account-b", body: clean, ready: true},
	}
	frames := []string{transportDelta04383, transportCompleted04383}
	owners := []string{"account-a", "account-b"}
	if err := verifyTransportRecovery04383("full-history", owners, base, frames, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func([]transportAttempt04383, []string, []string) ([]transportAttempt04383, []string, []string, bool)
	}{
		{"invalid auth", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			return a, h, f, true
		}},
		{"missing Ready evidence", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[0].ready = false
			return a, h, f, false
		}},
		{"wrong initial owner", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			h[0] = "account-b"
			return a, h, f, false
		}},
		{"wrong recovery owner", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].owner = "account-a"
			return a, h, f, false
		}},
		{"extra handshake", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			return a, append(h, "account-a"), f, false
		}},
		{"extra model replay", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			return append(a, a[1]), h, f, false
		}},
		{"stale header", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].turn = "turn-a"
			return a, h, f, false
		}},
		{"stale nested metadata", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].body = strings.Replace(a[1].body, `"client_metadata":{`, `"client_metadata":{"x-codex-turn-state":"turn-a",`, 1)
			return a, h, f, false
		}},
		{"rounded integer", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].body = strings.ReplaceAll(a[1].body, "9007199254740993", "9007199254740992")
			return a, h, f, false
		}},
		{"rounded uint64", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].body = strings.ReplaceAll(a[1].body, "18446744073709551615", "18446744073709552000")
			return a, h, f, false
		}},
		{"damaged tool output", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			a[1].body = strings.ReplaceAll(a[1].body, `"output":"done"`, `"output":"lost"`)
			return a, h, f, false
		}},
		{"leaked prelude", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			return a, h, append([]string{`{"type":"response.created"}`}, f...), false
		}},
		{"wrong terminal identity", func(a []transportAttempt04383, h, f []string) ([]transportAttempt04383, []string, []string, bool) {
			f[1] = strings.ReplaceAll(f[1], "resp-transport-recovered", "resp-other")
			return a, h, f, false
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, h, f, invalid := tc.mutate(append([]transportAttempt04383(nil), base...), append([]string(nil), owners...), append([]string(nil), frames...))
			if err := verifyTransportRecovery04383("full-history", h, a, f, invalid); err == nil {
				t.Fatal("material transport mutation escaped the oracle")
			}
		})
	}
}

func TestProdex04383MissingResponseOracleKeepsOpaqueTurnOnReplay(t *testing.T) {
	base := []transportAttempt04383{
		{owner: "account-a", turn: "turn-a", body: transportHistory04383, ready: true},
		{owner: "account-b", turn: "turn-a", body: transportHistory04383, ready: true},
	}
	for _, mutation := range []string{"none", "header", "metadata"} {
		attempts := append([]transportAttempt04383(nil), base...)
		if mutation == "header" {
			attempts[1].turn = ""
		}
		if mutation == "metadata" {
			attempts[1].body = strings.ReplaceAll(transportHistory04383, `"x-codex-turn-state":"turn-a",`, "")
		}
		err := verifyTransportRecovery04383("turn-missing", []string{"account-a", "account-b"}, attempts, []string{transportDelta04383, transportCompleted04383}, false)
		if (err == nil) != (mutation == "none") {
			t.Fatalf("missing-response turn mutation %s: %v", mutation, err)
		}
	}
}

func TestProdex04383TransportNegativeOracleRejectsReplayAndEmptyTerminal(t *testing.T) {
	for scenario, body := range map[string]string{
		"delta-only":       `{"type":"response.create","input":[{"type":"function_call_output","call_id":"call-large","output":"done"}]}`,
		"unknown-previous": strings.Replace(transportHistory04383, `"model":"gpt-6",`, `"model":"gpt-6","previous_response_id":"resp-unknown",`, 1),
	} {
		t.Run(scenario, func(t *testing.T) {
			const terminal = `{"error":{"code":"service_unavailable","message":"Runtime proxy could not secure a healthy upstream profile before the pre-commit retry budget was exhausted. Retry the request."},"status":503,"type":"error"}`
			base := transportAttempt04383{owner: "account-a", turn: "turn-a", body: body, ready: true}
			if err := verifyTransportNegative04383(scenario, body, []string{"account-a"}, []transportAttempt04383{base}, terminal, false); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name, frame string
				attempts    []transportAttempt04383
			}{
				{"incorrect owner reset", `{"error":{"code":"previous_response_not_found","message":"Previous response was not found. Retrying the full request."},"status":400,"type":"error"}`, []transportAttempt04383{base}},
				{"empty terminal", "", []transportAttempt04383{base}},
				{"wrong status", strings.Replace(terminal, "503", "502", 1), []transportAttempt04383{base}},
				{"model replay", terminal, []transportAttempt04383{base, base}},
				{"no attempt", terminal, nil},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if err := verifyTransportNegative04383(scenario, body, []string{"account-a"}, tc.attempts, tc.frame, false); err == nil {
						t.Fatal("negative control mutation escaped the oracle")
					}
				})
			}
		})
	}
}

func TestProdex04383UnboundQuotaDeltaKeepsExact429WithoutReplay(t *testing.T) {
	const terminal = `{"type":"error","status":429,"error":{"code":"insufficient_quota","message":"synthetic quota"}}`
	const body = `{"type":"response.create","input":[{"type":"function_call_output","call_id":"call-large","output":"done"}]}`
	base := transportAttempt04383{owner: "account-a", turn: "turn-a", body: body, ready: true}
	if err := verifyTransportNegative04383("unbound-quota-delta", body, []string{"account-a"}, []transportAttempt04383{base}, terminal, false); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{"", strings.Replace(terminal, "429", "503", 1)} {
		if err := verifyTransportNegative04383("unbound-quota-delta", body, []string{"account-a"}, []transportAttempt04383{base}, payload, false); err == nil {
			t.Fatalf("quota delta terminal mutation accepted: %s", payload)
		}
	}
	if err := verifyTransportNegative04383("unbound-quota-delta", body, []string{"account-a"}, []transportAttempt04383{base, base}, terminal, false); err == nil {
		t.Fatal("unbound quota delta model replay accepted")
	}
}
