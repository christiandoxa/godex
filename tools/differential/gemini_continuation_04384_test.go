package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestProdex04384GeminiContinuationOracleRejectsMatchingMutations(t *testing.T) {
	if !geminiContinuationNegativeControl04384() {
		t.Fatal("matching Gemini continuation corruptions escaped strict invariants")
	}
}

func TestProdex04384GeminiRestartPreservesSeedArguments(t *testing.T) {
	for _, fixture := range []struct{ scenario, key, endpoint string }{
		{geminiRestartResponseCase04384, apiKey, "http://127.0.0.1:1234/v1beta"},
		{geminiRestartKeyCase04384, rotationSecondaryKey, "http://127.0.0.1:1234/v1beta"},
		{geminiRestartEndpointCase04384, apiKey, "http://127.0.0.1:1234/v1beta/different"},
	} {
		t.Run(fixture.scenario, func(t *testing.T) {
			seed := []string{"super", "--api-key", apiKey, "--base-url", "http://127.0.0.1:1234/v1beta", "exec", "same request"}
			before := append([]string(nil), seed...)
			got := geminiRestartArguments04384(fixture.scenario, seed)
			want := []string{"super", "--api-key", fixture.key, "--base-url", fixture.endpoint, "exec", "same request"}
			if !reflect.DeepEqual(seed, before) || !reflect.DeepEqual(got, want) {
				t.Fatal("restart changed seed arguments or unrelated launch flags")
			}
		})
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
