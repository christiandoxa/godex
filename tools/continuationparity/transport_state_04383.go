package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func seedTransportAlternate04383(p product) error {
	name := "routing.json"
	if p.name == "prodex" {
		name = "runtime-continuations.json"
	}
	path := filepath.Join(p.state, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if p.name == "prodex" {
		value := state["value"].(map[string]any)
		value["response_profile_bindings"].(map[string]any)["resp-ready-b"] = map[string]any{"profile_name": "second", "bound_at": time.Now().Unix()}
		value["statuses"].(map[string]any)["response"].(map[string]any)["resp-ready-b"] = map[string]any{"state": "Verified", "confidence": 4, "last_verified_at": time.Now().Unix(), "last_touched_at": time.Now().Unix(), "last_verified_route": "responses", "success_count": 1}
	} else {
		var accounts struct{ Accounts []struct{ ID, Name string } }
		data, err := os.ReadFile(filepath.Join(p.state, "state.json"))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &accounts); err != nil {
			return err
		}
		id := ""
		for _, account := range accounts.Accounts {
			if account.Name == "second" {
				id = account.ID
			}
		}
		if id == "" {
			return errors.New("alternate account missing")
		}
		digest := sha256.Sum256([]byte("previous:resp-ready-b"))
		state["bindings"] = append(state["bindings"].([]any), map[string]any{"kind": "previous", "key": hex.EncodeToString(digest[:]), "account_id": id, "updated_unix": time.Now().Unix()})
	}
	return writeJSONFile(path, state)
}

func verifyTransportDurable04383(p product, scenario, recoveryEvent string) error {
	wantTurn := ""
	if scenario == "turn-missing" {
		wantTurn = "main"
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		owners, err := readTransportOwners04383(p)
		if err == nil && owners["turn"] == wantTurn && owners["response"] == "second" && owners["unrelated"] == "main" && owners["session"] == "__prodex_hard_binding_conflict__" {
			if p.name == "prodex" {
				if err := verifyTransportReadyLog04383(p, recoveryEvent); err != nil {
					return err
				}
			}
			fmt.Printf("%s durable transport snapshot: original turn=%q; recovered response=second; unrelated response=main; session snapshot=%q\n", p.name, owners["turn"], owners["session"])
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("durable transport owner mutation: owners=%v err=%v", owners, err)
		case <-ticker.C:
		}
	}
}

func readTransportOwners04383(p product) (map[string]string, error) {
	owners := make(map[string]string)
	if p.name == "prodex" {
		raw, err := os.ReadFile(filepath.Join(p.state, "runtime-continuations.json"))
		if err != nil {
			return nil, err
		}
		type binding struct {
			Profile string `json:"profile_name"`
		}
		var snapshot struct {
			Value struct {
				Responses map[string]binding `json:"response_profile_bindings"`
				Turns     map[string]binding `json:"turn_state_bindings"`
				Sessions  map[string]binding `json:"session_id_bindings"`
			} `json:"value"`
		}
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return nil, err
		}
		owners["turn"] = snapshot.Value.Turns["turn-a"].Profile
		owners["response"] = snapshot.Value.Responses["resp-transport-recovered"].Profile
		owners["unrelated"] = snapshot.Value.Responses["resp-main"].Profile
		owners["session"] = snapshot.Value.Sessions["sess-transport"].Profile
		return owners, nil
	}
	raw, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return nil, err
	}
	var state struct{ Accounts []struct{ ID, Name string } }
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	names := make(map[string]string)
	for _, account := range state.Accounts {
		names[account.ID] = account.Name
	}
	names["__prodex_hard_binding_conflict__"] = "__prodex_hard_binding_conflict__"
	raw, err = os.ReadFile(filepath.Join(p.state, "routing.json"))
	if err != nil {
		return nil, err
	}
	var snapshot struct {
		Bindings []struct {
			Key     string
			Account string `json:"account_id"`
		}
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	for label, value := range map[string]string{"turn": "turn:turn-a", "response": "previous:resp-transport-recovered", "unrelated": "previous:resp-main", "session": "session:sess-transport"} {
		digest := sha256.Sum256([]byte(value))
		for _, binding := range snapshot.Bindings {
			if binding.Key == hex.EncodeToString(digest[:]) {
				owner, ok := names[binding.Account]
				if !ok {
					return nil, errors.New("unregistered durable transport owner")
				}
				owners[label] = owner
			}
		}
	}
	return owners, nil
}

func verifyTransportNegativeOwners04383(p product, scenario string) error {
	wantHard := "main"
	if scenario == "previous-response" {
		wantHard = ""
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		owners, err := readTransportOwners04383(p)
		if err == nil && owners["session"] == "main" && owners["turn"] == wantHard && owners["unrelated"] == wantHard && owners["response"] == "" {
			fmt.Printf("%s %s durable snapshot: session=main; original hard bindings=%q; no alternate response\n", p.name, scenario, wantHard)
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("%s changed durable signal ownership: owners=%v err=%v", scenario, owners, err)
		case <-ticker.C:
		}
	}
}

func verifyTransportReadyLog04383(p product, recoveryEvent string) error {
	recoveryMarker := recoveryEvent + " profile=main affinity_released=true"
	if recoveryEvent == "previous_response_not_found" {
		recoveryMarker = recoveryEvent + " profile=main retry_index=0 replay_turn_state=None"
	}
	paths, err := filepath.Glob(filepath.Join(p.state, "logs", "prodex-runtime-*.log"))
	if err != nil {
		return err
	}
	for _, path := range paths[:min(len(paths), 4)] {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		recovery, ready := false, false
		for _, line := range strings.Split(string(data), "\n") {
			recovery = recovery || strings.Contains(line, recoveryMarker)
			ready = ready || (strings.Contains(line, "selection_keep_current route=websocket profile=second") && strings.Contains(line, "quota_source=probe_cache") && strings.Contains(line, "quota_band=quota_healthy"))
		}
		if recovery && ready {
			return nil
		}
	}
	return errors.New("official runtime log lacks failed main owner and authenticated Ready alternate selection")
}

func transportProfilesReady04383(p product, b *transportBackend04383) bool {
	b.mu.Lock()
	ready := b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0 && b.warmups == 2 && !b.invalid
	b.mu.Unlock()
	if !ready || p.name != "prodex" {
		return ready
	}
	// A server receipt precedes Prodex applying the asynchronous quota result.
	paths, err := filepath.Glob(filepath.Join(p.state, "logs", "prodex-runtime-*.log"))
	if err != nil {
		return false
	}
	for _, path := range paths[:min(len(paths), 4)] {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		log := string(data)
		if strings.Contains(log, "profile_probe_refresh_ok profile=main ") && strings.Contains(log, "profile_probe_refresh_ok profile=second ") {
			return true
		}
	}
	return false
}
