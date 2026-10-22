package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProdex04384GeminiStateRequiresObservedResponseAndSession(t *testing.T) {
	run := geminiCandidateResult{Client: exchange{Status: 200, Body: `{"id":"response-fixture"}`},
		Upstream: []geminiCapturedRequest{{Slot: "single", Auth: true}}}
	owners := geminiContinuationOwners04384("raw-key-session-limit", run, "http://127.0.0.1:1234/v1beta")
	session, _, _ := geminiRawKeyIdentity04384("raw-key-session-limit")
	for _, fixture := range []struct{ kind, value string }{{"previous", "response-fixture"}, {"session", session}} {
		t.Run(fixture.kind, func(t *testing.T) {
			digest := sha256.Sum256([]byte(fixture.kind + ":" + fixture.value))
			key := hex.EncodeToString(digest[:])
			body := fmt.Sprintf(`{"version":1,"bindings":[{"kind":%q,"key":%q,"account_id":%q,"updated_unix":%d}]}`,
				fixture.kind, key, fixtureConflictOwner, time.Now().Unix())
			if !validFixtureRoutingBindings([]byte(body), owners) {
				t.Fatal("authenticated accepted fixture binding was rejected")
			}
			for _, mutation := range []string{
				strings.Replace(body, key, strings.Repeat("a", 64), 1),
				strings.Replace(body, fixtureConflictOwner, "credential-owner", 1),
				strings.Replace(body, `"kind":`+fmt.Sprintf("%q", fixture.kind), `"kind":"turn"`, 1),
			} {
				if validFixtureRoutingBindings([]byte(mutation), owners) {
					t.Fatal("unobserved identity or binding kind escaped the audit")
				}
			}
		})
	}
	owner := owners["response-fixture"]
	binding := fmt.Sprintf(`{"profile_name":"local","bound_at":%d,"binding_identity":{"provider":"gemini","credential_identity":%q,"endpoint_identity":%q}}`, time.Now().Unix(), owner.credential, owner.endpoint)
	state := fmt.Sprintf(`{"generation":1,"value":{"response_profile_bindings":{"response-fixture":%s},"session_profile_bindings":{},"turn_state_bindings":{},"session_id_bindings":{%q:%s},"statuses":{"response":{},"turn_state":{},"session_id":{}}}}`, binding, session, binding)
	if !validFixtureProdexContinuations("state/runtime-continuations.json", []byte(state), owners) {
		t.Fatal("observed Gemini session identity was rejected")
	}
	for _, mutation := range []string{
		strings.ReplaceAll(state, `"gemini"`, `"deepseek"`),
		strings.ReplaceAll(state, owner.credential, fixtureBindingDigest("credential", rotationSecondaryKey)),
		strings.ReplaceAll(state, owner.endpoint, fixtureBindingDigest("endpoint", "http://127.0.0.1:5678")),
		strings.Replace(state, fmt.Sprintf("%q", session), `"unknown-session"`, 1),
	} {
		if validFixtureProdexContinuations("state/runtime-continuations.json", []byte(mutation), owners) {
			t.Fatal("foreign Gemini session identity escaped the audit")
		}
	}
}

func TestProdex04384GeminiStateRejectsUnverifiedSuccess(t *testing.T) {
	run := geminiCandidateResult{Client: exchange{Status: 200, Body: `{"id":"response-fixture"}`},
		Upstream: []geminiCapturedRequest{{Slot: "single"}}}
	if len(geminiContinuationOwners04384("buffered-body-under", run, "http://127.0.0.1:1234/v1beta")) != 0 {
		t.Fatal("unauthenticated observation authorized durable state")
	}
	run.Upstream[0].Auth, run.Client.Status = true, 503
	if len(geminiContinuationOwners04384("raw-key-session-limit", run, "http://127.0.0.1:1234/v1beta")) != 0 {
		t.Fatal("rejected response authorized session state")
	}
}
