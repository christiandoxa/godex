package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestProdex04379WebSocketDifferentialRejectsMaterialDrift(t *testing.T) {
	canonical := websocketExpectation{
		mode:     "ws-keepalive",
		want:     []string{"response.output_text.delta", "response.completed"},
		delta:    "recovered",
		attempts: 2,
		owner:    "account-a",
	}
	success := wsProbe{
		types: []string{"response.output_text.delta", "response.completed"},
		delta: []string{"recovered", ""},
		body: []string{
			`{"type":"response.output_text.delta","delta":"recovered"}`,
			`{"type":"response.completed","response":{"id":"resp-recovered"}}`,
		},
	}
	owners := []string{"account-a", "account-a"}
	if err := verifyWebsocketExpectation(canonical, success, owners); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		changed   wsProbe
		attempts  []string
		errorText string
	}{
		{"missing recovery attempt", success, []string{"account-a"}, "upstream attempts"},
		{"wrong account without attempt drift", success, []string{"account-a", "account-b"}, "canonical owner"},
		{"leaked heartbeat", wsProbe{
			types: []string{"keepalive", "response.output_text.delta", "response.completed"},
			delta: []string{"", "recovered", ""},
		}, owners, "event kinds"},
		{"wrong event order", wsProbe{
			types: []string{"response.completed", "response.output_text.delta"},
			delta: []string{"", "recovered"},
		}, owners, "event kinds"},
		{"missing visible output", wsProbe{
			types: success.types, delta: []string{"other", ""},
		}, owners, "missing visible"},
		{"leaked discarded response ID", wsProbe{
			types: success.types, delta: success.delta,
			body: []string{`{"type":"response.output_text.delta","delta":"recovered","prior":"discarded-response"}`},
		}, owners, "leaked failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyWebsocketExpectation(canonical, tc.changed, tc.attempts)
			if err == nil || !strings.Contains(err.Error(), tc.errorText) {
				t.Fatalf("drift not detected: error=%v, want %q", err, tc.errorText)
			}
		})
	}
	committed := websocketExpectation{
		mode:  "ws-committed",
		want:  []string{"keepalive", "response.output_text.delta", "response.failed"},
		delta: "visible", attempts: 1, owner: "account-a",
	}
	committedResult := wsProbe{
		types: committed.want,
		delta: []string{"", "visible", ""},
		body: []string{
			`{"type":"keepalive"}`,
			`{"type":"response.output_text.delta","delta":"visible"}`,
			`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`,
		},
	}
	if err := verifyWebsocketExpectation(committed, committedResult, []string{"account-a"}); err != nil {
		t.Fatal(err)
	}
	if err := verifyWebsocketExpectation(committed, committedResult, []string{"account-a", "account-b"}); err == nil {
		t.Fatal("post-commit replay across upstream profiles was accepted")
	}
}

func TestProdex04379WebSocketOracleRejectsDirtyOrStaleGoBuild(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	good := []debug.BuildSetting{
		{Key: "vcs.revision", Value: commit},
		{Key: "vcs.modified", Value: "false"},
	}
	if err := verifyCandidateVCS(good, commit); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		settings []debug.BuildSetting
	}{
		{"dirty", []debug.BuildSetting{{Key: "vcs.revision", Value: commit}, {Key: "vcs.modified", Value: "true"}}},
		{"stale", []debug.BuildSetting{{Key: "vcs.revision", Value: "0000000000000000000000000000000000000000"}, {Key: "vcs.modified", Value: "false"}}},
		{"missing", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := verifyCandidateVCS(tc.settings, commit); err == nil {
				t.Fatal("unqualified candidate unexpectedly accepted")
			}
		})
	}
}

func TestProdex04379WebSocketUnboundStaleOracleRejectsRewrites(t *testing.T) {
	good := wsProbe{
		types: []string{"error"},
		body:  []string{`{"type":"error","status":409,"error":{"code":"stale_continuation"}}`},
	}
	probe := websocketExpectation{
		mode: "ws-unknown-missing", want: []string{"error"}, attempts: 4,
		owner: "account-a", errorCode: "stale_continuation", errorStatus: 409,
	}
	owners := []string{"account-a", "account-a", "account-a", "account-a"}
	if err := verifyWebsocketExpectation(probe, good, owners); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.body = []string{`{"type":"error","status":503,"error":{"code":"stale_continuation"}}`}
	if err := verifyWebsocketExpectation(probe, bad, owners); err == nil {
		t.Fatal("mismatched stale HTTP status was accepted")
	}
	bad.body = []string{`{"type":"error","status":409,"error":{"code":"other_error"}}`}
	if err := verifyWebsocketExpectation(probe, bad, owners); err == nil {
		t.Fatal("mismatched stale error code was accepted")
	}
	body := `{"type":"response.create","mode":"ws-unknown-missing","previous_response_id":"resp-tool-missing","session_id":"sess-conflict","input":[{"type":"function_call_output","call_id":"call-missing","output":"done"}]}`
	backend := &backend{attempts: []attempt{{mode: probe.mode, account: "account-a", body: body}}}
	if err := verifyWebsocketOpaqueChain(backend, 0, probe.mode); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{
		strings.Replace(body, "resp-tool-missing", "resp-wrong", 1),
		strings.Replace(body, "call-missing", "call-wrong", 1),
		strings.Replace(body, `"output":"done"`, `"output":""`, 1),
		strings.Replace(body, "function_call_output", "message", 1),
		`{"input":[]}`,
	} {
		backend.attempts[0].body = mutation
		if err := verifyWebsocketOpaqueChain(backend, 0, probe.mode); err == nil {
			t.Fatalf("rewritten opaque continuation accepted: %s", mutation)
		}
	}
}

func TestProdex04379BoundMissingWebsocketOraclePreservesToolContext(t *testing.T) {
	body := `{"type":"response.create","mode":"ws-bound-missing","previous_response_id":"resp-main","session_id":"sess-conflict","input":[{"type":"function_call_output","call_id":"call-bound","output":"done"}]}`
	backend := &backend{attempts: []attempt{{mode: "ws-bound-missing", account: "account-a", body: body}}}
	if err := verifyWebsocketOpaqueChain(backend, 0, "ws-bound-missing"); err != nil {
		t.Fatal(err)
	}
	for _, altered := range []string{
		strings.Replace(body, "resp-main", "resp-different", 1),
		strings.Replace(body, "call-bound", "call-other", 1),
		strings.Replace(body, `"output":"done"`, `"output":null`, 1),
	} {
		backend.attempts[0].body = altered
		if err := verifyWebsocketOpaqueChain(backend, 0, "ws-bound-missing"); err == nil {
			t.Fatalf("bound tool-output rewrite accepted: %s", altered)
		}
	}
}
