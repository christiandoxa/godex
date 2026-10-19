package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
					"response_profile_bindings": map[string]any{"resp-main": map[string]string{"profile_name": "main"}, "resp-transport-recovered": map[string]string{"profile_name": "second"}},
					"session_id_bindings":       map[string]any{"sess-transport": map[string]string{"profile_name": conflict}},
				}}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writeJSONFile(filepath.Join(p.state, "state.json"), map[string]any{"accounts": []map[string]string{{"id": "a", "name": "main"}, {"id": "b", "name": "second"}}}); err != nil {
					t.Fatal(err)
				}
				var bindings []map[string]string
				for value, owner := range map[string]string{"previous:resp-main": "a", "previous:resp-transport-recovered": "b", "session:sess-transport": conflict} {
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
			if owners["turn"] != "" || owners["response"] != "second" || owners["unrelated"] != "main" || owners["session"] != conflict {
				t.Fatalf("durable ownership changed: %v", owners)
			}
			delete(owners, "turn")
			if !reflect.DeepEqual(owners, map[string]string{"response": "second", "unrelated": "main", "session": conflict}) {
				t.Fatalf("unexpected durable ownership: %v", owners)
			}
		})
	}
}
