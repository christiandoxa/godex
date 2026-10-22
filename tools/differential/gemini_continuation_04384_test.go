package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestProdex04384GeminiContinuationOracleRejectsMatchingMutations(t *testing.T) {
	if !geminiContinuationNegativeControl04384() {
		t.Fatal("matching Gemini continuation corruptions escaped strict invariants")
	}
}

func TestProdex04384GeminiContinuationOwnerPinsProvider(t *testing.T) {
	run := geminiCandidateResult{Client: exchange{Status: 200, Body: `{"id":"chatcmpl-differential"}`},
		Upstream: []geminiCapturedRequest{{Slot: "single"}}}
	owners := geminiContinuationOwners04384(geminiUnboundResponseCase04384, run, "http://127.0.0.1:1234/v1beta")
	owner := owners["chatcmpl-differential"]
	if len(owners) != 2 || owner.provider != "gemini" || owners["chatcmpl-unknown"] != owner {
		t.Fatal("accepted Gemini request did not establish the exact fixture aliases")
	}
	encoded := fmt.Sprintf(`{"chatcmpl-differential":{"profile_name":"local","bound_at":%d,"binding_identity":{"provider":"gemini","credential_identity":%q,"endpoint_identity":%q}}}`,
		time.Now().Unix(), owner.credential, owner.endpoint)
	var bindings map[string]fixtureProdexBinding
	if json.Unmarshal([]byte(encoded), &bindings) != nil || !validFixtureProdexBindings(bindings, owners) {
		t.Fatal("observed Gemini ownership was rejected")
	}
	bindings["chatcmpl-differential"].Identity.Provider = "deepseek"
	if validFixtureProdexBindings(bindings, owners) {
		t.Fatal("foreign provider identity escaped the audit")
	}
}
