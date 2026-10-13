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
