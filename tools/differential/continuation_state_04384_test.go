package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProdex04384FixtureContinuationEvidence(t *testing.T) {
	prior := rawKeyProbeSeedOwner("http://127.0.0.1:1234/v1")
	for _, body := range []string{`{"id":"resp-new"}`, "data: {\"response\":{\"id\":\"resp-new\"}}\r\n\r\n"} {
		client := exchange{Status: 200, Body: body}
		if strings.HasPrefix(body, "data:") {
			client.Headers = map[string][]string{"Content-Type": {"text/event-stream"}}
		}
		owners := fixtureContinuationOwners(client, []upstreamRequest{{KeySlot: "primary"}}, "http://127.0.0.1:5678/v1", prior)
		if len(owners) != 2 || owners["resp-new"].credential == prior["chatcmpl-differential"].credential ||
			owners["resp-new"].endpoint == prior["chatcmpl-differential"].endpoint || len(prior) != 1 {
			t.Fatal("continuation evidence lost response/key/endpoint separation")
		}
	}
	if got := fixtureContinuationOwners(exchange{Status: 503, Body: `{"id":"unverified"}`}, []upstreamRequest{{KeySlot: "primary"}}, "http://127.0.0.1:1234/v1", nil); len(got) != 0 {
		t.Fatal("rejected response authorized a durable binding")
	}
}

func TestProdex04384DurableFixtureAcceptsOnlyObservedRejections(t *testing.T) {
	owners := rawKeyProbeSeedOwner("http://127.0.0.1:1234/v1")
	const response = "chatcmpl-differential"
	// Compute the Godex namespace independently from the provider identity namespace.
	digest := sha256.Sum256([]byte("previous:" + response))
	key := hex.EncodeToString(digest[:])
	valid := fmt.Sprintf(`{"version":1,"bindings":[{"kind":"previous","key":%q,"account_id":%q,"updated_unix":%d}]}`, key, fixtureConflictOwner, time.Now().Unix())
	if !validFixtureRoutingBindings([]byte(valid), owners) || validFixtureRoutingBindings([]byte(valid), nil) {
		t.Fatal("observed unavailable continuation was not distinguished from unknown state")
	}
	for _, mutation := range []string{
		strings.Replace(valid, key, strings.Repeat("a", 64), 1),
		strings.Replace(valid, fixtureConflictOwner, strings.Repeat("a", 32), 1),
		strings.Replace(valid, `"previous"`, `"session"`, 1),
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"bindings":`, `"extra":true,"bindings":`, 1),
	} {
		if validFixtureRoutingBindings([]byte(mutation), owners) {
			t.Fatal("unauthorized durable owner escaped fixture audit")
		}
	}
}

func TestProdex04384ContinuationSnapshotsPinEveryIdentityField(t *testing.T) {
	owners := rawKeyProbeSeedOwner("http://127.0.0.1:1234/v1")
	owner := owners["chatcmpl-differential"]
	binding := fmt.Sprintf(`{"profile_name":"local","bound_at":%d,"binding_identity":{"provider":"deepseek","credential_identity":%q,"endpoint_identity":%q}}`, time.Now().Unix(), owner.credential, owner.endpoint)
	continuations := `{"response_profile_bindings":{"chatcmpl-differential":` + binding + `},"session_profile_bindings":{},"turn_state_bindings":{},"session_id_bindings":{},"statuses":{"response":{},"turn_state":{},"session_id":{}}}`
	fixtures := map[string]string{
		"state/state.json":                        `{"schema_version":1,"active_profile":null,"profiles":{},"last_run_selected_at":{},"response_profile_bindings":{"chatcmpl-differential":` + binding + `},"session_profile_bindings":{}}`,
		"state/runtime-continuations.json":        `{"generation":1,"value":` + continuations + `}`,
		"state/runtime-continuation-journal.json": fmt.Sprintf(`{"generation":1,"value":{"saved_at":%d,"continuations":%s}}`, time.Now().Unix(), continuations),
	}
	for name, valid := range fixtures {
		t.Run(name, func(t *testing.T) {
			if !validFixtureProdexContinuations(name, []byte(valid), owners) {
				t.Fatal("observed public-identity snapshot rejected")
			}
			root := fixtureStateRoot(t)
			for _, suffix := range []string{"", ".last-good"} {
				path := filepath.Join(root, filepath.FromSlash(name+suffix))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if failures := auditFixtureDurableStateWithBindings(root, owners); len(failures) != 0 {
				t.Fatalf("known continuation failed durable audit: %v", failures)
			}
			if failures := auditFixtureDurableState(root); len(failures) == 0 {
				t.Fatal("unobserved continuation escaped the durable audit")
			}
			fenced := strings.Replace(valid, binding, fmt.Sprintf(`{"profile_name":%q,"bound_at":%d}`, fixtureConflictOwner, time.Now().Unix()), 1)
			if !validFixtureProdexContinuations(name, []byte(fenced), owners) {
				t.Fatal("unavailable-profile conflict rewrite rejected")
			}
			for _, mutation := range []string{
				strings.Replace(valid, owner.credential, fixtureBindingDigest("credential", rotationPrimaryKey), 1),
				strings.Replace(valid, owner.endpoint, fixtureBindingDigest("endpoint", "http://127.0.0.1:5678/v1"), 1),
				strings.Replace(valid, `"deepseek"`, `"gemini"`, 1),
				strings.Replace(valid, `"local"`, `"managed"`, 1),
				strings.Replace(valid, `"chatcmpl-differential"`, `"unobserved"`, 1),
				strings.Replace(valid, `"profile_name":`, `"hidden":true,"profile_name":`, 1),
				valid + `{}`,
			} {
				if validFixtureProdexContinuations(name, []byte(mutation), owners) {
					t.Fatal("wrong or corrupt binding identity escaped")
				}
			}
		})
	}
}
