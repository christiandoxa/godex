package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProdex04383UnboundRecoveryRequiresNoReleasedBindingAndReadySelection(t *testing.T) {
	p := product{name: "prodex", state: t.TempDir()}
	logs := filepath.Join(p.state, "logs")
	if err := os.Mkdir(logs, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "prodex-runtime-fixture.log")
	const event = "transport_failure_turn_state_full_context_replay"
	const log = event + " profile=main affinity_released=false\nselection_pick route=websocket profile=second mode=ready quota_source=probe_cache quota_band=quota_healthy\n"
	for _, field := range []string{"", "affinity_released=false", "profile=main", "mode=ready", "quota_source=probe_cache", "quota_band=quota_healthy"} {
		data := log
		if field != "" {
			data = strings.Replace(data, field, "mutated", 1)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		err := verifyTransportReadyLog04383(p, "unbound-history", event)
		if (err == nil) != (field == "") {
			t.Fatalf("unbound recovery evidence mutation %q: %v", field, err)
		}
	}
}

func TestProdex04383RecoveryLogRequiresMatchingFailureAndReadyCache(t *testing.T) {
	p := product{name: "prodex", state: t.TempDir()}
	logs := filepath.Join(p.state, "logs")
	if err := os.Mkdir(logs, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "prodex-runtime-fixture.log")
	for _, event := range []string{"transport_failure_turn_state_full_context_replay", "quota_blocked_turn_state_full_context_replay"} {
		log := event + " profile=main affinity_released=true\nselection_keep_current route=websocket profile=second quota_source=probe_cache quota_band=quota_healthy\n"
		if err := os.WriteFile(path, []byte(log), 0600); err != nil {
			t.Fatal(err)
		}
		if err := verifyTransportReadyLog04383(p, "full-history", event); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{event, "profile=main", "affinity_released=true", "profile=second", "quota_source=probe_cache", "quota_band=quota_healthy"} {
			if err := os.WriteFile(path, []byte(strings.Replace(log, field, "mutated", 1)), 0600); err != nil {
				t.Fatal(err)
			}
			if err := verifyTransportReadyLog04383(p, "full-history", event); err == nil {
				t.Fatalf("%s accepted mutated %s", event, field)
			}
		}
	}
}

func TestProdex04383TransportWaitsForAppliedQuotaProbes(t *testing.T) {
	p := product{name: "prodex", state: t.TempDir()}
	b := &transportBackend04383{receipts: map[string]int{"account-a": 1, "account-b": 1}, warmups: 2}
	logs := filepath.Join(p.state, "logs")
	if err := os.Mkdir(logs, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "prodex-runtime-fixture.log")
	for _, tc := range []struct {
		name, log string
		ready     bool
	}{
		{"received but not applied", "profile_probe_refresh_start profile=main\nprofile_probe_refresh_start profile=second\n", false},
		{"only main applied", "profile_probe_refresh_ok profile=main lag_ms=1\n", false},
		{"alternate failed", "profile_probe_refresh_ok profile=main lag_ms=1\nprofile_probe_refresh_error profile=second lag_ms=1\n", false},
		{"both applied", "profile_probe_refresh_ok profile=main lag_ms=1\nprofile_probe_refresh_ok profile=second lag_ms=1\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.log), 0600); err != nil {
				t.Fatal(err)
			}
			if got := transportProfilesReady04383(p, b); got != tc.ready {
				t.Fatalf("Ready=%t, want %t after %s", got, tc.ready, tc.name)
			}
		})
	}
	b.invalid = true
	if transportProfilesReady04383(p, b) {
		t.Fatal("applied quota cannot excuse invalid authentication")
	}
}

func TestProdex04383TransportDurableProjectionKeepsConflictAndResponseOwners(t *testing.T) {
	for _, name := range []string{"prodex", "godex"} {
		t.Run(name, func(t *testing.T) {
			p := product{name: name, state: t.TempDir()}
			const conflict = "__prodex_hard_binding_conflict__"
			if name == "prodex" {
				if err := writeJSONFile(filepath.Join(p.state, "runtime-continuations.json"), map[string]any{"generation": 2, "value": map[string]any{
					"turn_state_bindings":       map[string]any{},
					"response_profile_bindings": map[string]any{"resp-main": map[string]string{"profile_name": "main"}, "resp-transport-recovered": map[string]string{"profile_name": "second"}, "resp-unknown": map[string]string{"profile_name": "second"}},
					"session_id_bindings":       map[string]any{"sess-transport": map[string]string{"profile_name": conflict}},
				}}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writeJSONFile(filepath.Join(p.state, "state.json"), map[string]any{"accounts": []map[string]string{{"id": "a", "name": "main"}, {"id": "b", "name": "second"}}}); err != nil {
					t.Fatal(err)
				}
				var bindings []map[string]string
				for value, owner := range map[string]string{"previous:resp-unknown": "b", "previous:resp-main": "a", "previous:resp-transport-recovered": "b", "session:sess-transport": conflict} {
					digest := sha256.Sum256([]byte(value))
					bindings = append(bindings, map[string]string{"key": hex.EncodeToString(digest[:]), "account_id": owner})
				}
				if err := writeJSONFile(filepath.Join(p.state, "routing.json"), map[string]any{"bindings": bindings}); err != nil {
					t.Fatal(err)
				}
			}
			owners, err := readTransportOwners04383(p)
			if err != nil {
				t.Fatal(err)
			}
			if owners["unknown"] != "second" || owners["turn"] != "" || owners["response"] != "second" || owners["unrelated"] != "main" || owners["session"] != conflict {
				t.Fatalf("durable ownership changed: %v", owners)
			}
			delete(owners, "turn")
			if !reflect.DeepEqual(owners, map[string]string{"unknown": "second", "response": "second", "unrelated": "main", "session": conflict}) {
				t.Fatalf("unexpected durable ownership: %v", owners)
			}
		})
	}
}
