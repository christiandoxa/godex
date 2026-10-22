package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const fixtureConflictOwner = "__prodex_hard_binding_conflict__"

type fixtureContinuationOwner struct{ credential, endpoint, provider, kind string }

func fixtureContinuationOwners(client exchange, upstream []upstreamRequest, endpoint string, prior map[string]fixtureContinuationOwner) map[string]fixtureContinuationOwner {
	owners := make(map[string]fixtureContinuationOwner, len(prior)+1)
	for id, owner := range prior {
		owners[id] = owner
	}
	if client.Status < 200 || client.Status >= 300 || len(upstream) == 0 {
		return owners
	}
	credential := map[string]string{"single": apiKey, "primary": rotationPrimaryKey, "secondary": rotationSecondaryKey}[upstream[len(upstream)-1].KeySlot]
	if credential == "" {
		return owners
	}
	owner := fixtureContinuationOwner{credential: fixtureBindingDigest("credential", credential), endpoint: fixtureBindingDigest("endpoint", endpoint), provider: "deepseek", kind: "previous"}
	parts := []string{client.Body}
	if strings.Contains(client.Headers.Get("Content-Type"), "text/event-stream") {
		parts = nil
		for _, line := range strings.Split(client.Body, "\n") {
			if strings.HasPrefix(line, "data: ") {
				parts = append(parts, strings.TrimPrefix(line, "data: "))
			}
		}
	}
	for _, part := range parts {
		var value struct {
			ID       string `json:"id"`
			Response struct {
				ID string `json:"id"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(part), &value) != nil {
			continue
		}
		for _, id := range []string{value.ID, value.Response.ID} {
			if id != "" {
				owners[id] = owner
			}
		}
	}
	return owners
}

// Fixture URLs are already canonical ASCII loopback URLs. This is not a
// replacement for Rust URL normalization in the production identity policy.
func fixtureBindingDigest(kind, value string) string {
	digest := sha256.Sum256([]byte("prodex-runtime-binding-v1\x00" + kind + "\x00" + value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func fixturePreviousDigest(value string) string {
	digest := sha256.Sum256([]byte("previous:" + value))
	return hex.EncodeToString(digest[:])
}

func validFixtureRoutingBindings(data []byte, expected map[string]fixtureContinuationOwner) bool {
	var snapshot struct {
		Version  int `json:"version"`
		Bindings []struct {
			Kind        string `json:"kind"`
			Key         string `json:"key"`
			AccountID   string `json:"account_id"`
			UpdatedUnix int64  `json:"updated_unix"`
		} `json:"bindings"`
	}
	if !strictFixtureStateJSON(data, &snapshot) || snapshot.Version != 1 || snapshot.Bindings == nil {
		return false
	}
	seen := make(map[string]bool)
	for _, binding := range snapshot.Bindings {
		known := false
		for id, owner := range expected {
			digest := sha256.Sum256([]byte(owner.kind + ":" + id))
			known = known || binding.Kind == owner.kind && binding.Key == hex.EncodeToString(digest[:])
		}
		if !known || (binding.Kind != "previous" && binding.Kind != "session") || binding.AccountID != fixtureConflictOwner ||
			!fixtureBindingTimestamp(binding.UpdatedUnix) || seen[binding.Key] {
			return false
		}
		seen[binding.Key] = true
	}
	return true
}

type fixtureProdexBinding struct {
	ProfileName string `json:"profile_name"`
	BoundAt     int64  `json:"bound_at"`
	Identity    *struct {
		Provider   string `json:"provider"`
		Credential string `json:"credential_identity"`
		Endpoint   string `json:"endpoint_identity"`
	} `json:"binding_identity,omitempty"`
}

type fixtureProdexContinuations struct {
	Responses map[string]fixtureProdexBinding `json:"response_profile_bindings"`
	Sessions  map[string]json.RawMessage      `json:"session_profile_bindings"`
	Turns     map[string]json.RawMessage      `json:"turn_state_bindings"`
	SessionID map[string]fixtureProdexBinding `json:"session_id_bindings"`
	Statuses  struct {
		Response map[string]json.RawMessage `json:"response"`
		Turn     map[string]json.RawMessage `json:"turn_state"`
		Session  map[string]json.RawMessage `json:"session_id"`
	} `json:"statuses"`
}

func validFixtureProdexContinuations(name string, data []byte, expected map[string]fixtureContinuationOwner) bool {
	name = strings.TrimSuffix(name, ".last-good")
	if name == "state/state.json" {
		var state struct {
			Version   int                             `json:"schema_version"`
			Active    *string                         `json:"active_profile"`
			Profiles  map[string]json.RawMessage      `json:"profiles"`
			Selected  map[string]json.RawMessage      `json:"last_run_selected_at"`
			Responses map[string]fixtureProdexBinding `json:"response_profile_bindings"`
			Sessions  map[string]json.RawMessage      `json:"session_profile_bindings"`
		}
		return strictFixtureStateJSON(data, &state) && state.Version == 1 && state.Active == nil &&
			emptyFixtureStateMap(state.Profiles) && emptyFixtureStateMap(state.Selected) &&
			emptyFixtureStateMap(state.Sessions) && validFixtureProdexBindings(state.Responses, expected, "previous")
	}
	var envelope struct {
		Generation uint64          `json:"generation"`
		Value      json.RawMessage `json:"value"`
	}
	if !strictFixtureStateJSON(data, &envelope) || envelope.Generation == 0 {
		return false
	}
	var value fixtureProdexContinuations
	if name == "state/runtime-continuation-journal.json" {
		var journal struct {
			SavedAt       int64                      `json:"saved_at"`
			Continuations fixtureProdexContinuations `json:"continuations"`
		}
		if !strictFixtureStateJSON(envelope.Value, &journal) || !fixtureBindingTimestamp(journal.SavedAt) {
			return false
		}
		value = journal.Continuations
	} else if !strictFixtureStateJSON(envelope.Value, &value) {
		return false
	}
	return validFixtureProdexBindings(value.Responses, expected, "previous") &&
		emptyFixtureStateMap(value.Sessions) && emptyFixtureStateMap(value.Turns) && validFixtureProdexBindings(value.SessionID, expected, "session") &&
		emptyFixtureStateMap(value.Statuses.Response) && emptyFixtureStateMap(value.Statuses.Turn) && emptyFixtureStateMap(value.Statuses.Session)
}

func validFixtureProdexBindings(bindings map[string]fixtureProdexBinding, expected map[string]fixtureContinuationOwner, kind string) bool {
	if bindings == nil {
		return false
	}
	for id, binding := range bindings {
		owner, known := expected[id]
		if !known || owner.kind != kind || !fixtureBindingTimestamp(binding.BoundAt) {
			return false
		}
		if binding.ProfileName == fixtureConflictOwner && binding.Identity == nil {
			continue
		}
		if binding.ProfileName != "local" || binding.Identity == nil || binding.Identity.Provider != owner.provider ||
			binding.Identity.Credential != owner.credential || binding.Identity.Endpoint != owner.endpoint {
			return false
		}
	}
	return true
}

func fixtureBindingTimestamp(value int64) bool { return value > 0 && value <= time.Now().Unix()+5 }

func emptyFixtureStateMap(value map[string]json.RawMessage) bool {
	return value != nil && len(value) == 0
}

func strictFixtureStateJSON(data []byte, value any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value) == nil && decoder.Decode(new(any)) == io.EOF
}
