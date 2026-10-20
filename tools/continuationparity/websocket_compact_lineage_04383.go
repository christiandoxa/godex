package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

// A Compact request establishes a separate durable turn lineage. The tagged
// 0.438.3 WebSocket selector honors that lineage even though its request's
// quota route remains WebSocket. The unbound control must stay on main.
const compactLineageMessage04383 = `{"type":"response.create","model":"gpt-6","x-codex-turn-state":"compact-turn","input":[{"type":"message","role":"user","content":"first"},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"followup"}]}`

func runWebSocketCompactLineageProduct04383(root string, p product) error {
	for _, scenario := range []string{"compact-lineage", "compact-unbound"} {
		if err := runCompactLineageCase04383(filepath.Join(root, scenario), p, scenario); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	return nil
}

func runCompactLineageCase04383(root string, p product, scenario string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, scenario); err != nil {
		return err
	}
	initial, err := readTransportOwners04383(p)
	if err != nil {
		return err
	}
	initialLineage, err := readCompactLineageOwner04383(p)
	if err != nil {
		return err
	}
	wantOwner := "second"
	if scenario == "compact-unbound" {
		wantOwner = "main"
	}
	wantLineage := ""
	if scenario == "compact-lineage" {
		wantLineage = "second"
	}
	if initialLineage != wantLineage || initial["turn"] != "main" || initial["unrelated"] != "main" || initial["session"] != "main" {
		return fmt.Errorf("invalid initial Compact lineage fixture: lineage=%q main_turn=%q main_response=%q session=%q",
			initialLineage, initial["turn"], initial["unrelated"], initial["session"])
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383(scenario)
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 5 * time.Second}
	// Warm second first and main last: without a Compact alias the active
	// profile must be main. This makes a second-profile decision attributable
	// to the explicit Compact lineage, not to last-writer/active selection.
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("Compact owner warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	readyDeadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(readyDeadline) {
			return errors.New("Compact lineage fixture has no applied authenticated Ready quota receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	connection, reader, err := wsConnectHeaders04383(endpoint+"/responses", "")
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(12 * time.Second))
	if err := websocketframe.WriteFrame(connection, 1, []byte(compactLineageMessage04383), true); err != nil {
		return err
	}
	frames := make([]string, 0, 2)
	for len(frames) < 2 {
		opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
		if err != nil {
			return fmt.Errorf("Compact lineage client: %w; frames=%q", err, frames)
		}
		if opcode != 1 {
			return fmt.Errorf("unexpected Compact lineage client opcode %d", opcode)
		}
		frames = append(frames, string(payload))
	}
	backend.mu.Lock()
	err = verifyCompactLineage04383(wantOwner, backend.handshakes, backend.attempts, frames, backend.invalid)
	backend.mu.Unlock()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		owners, err := readTransportOwners04383(p)
		lineage, lineageErr := readCompactLineageOwner04383(p)
		if err == nil && lineageErr == nil && lineage == wantLineage && owners["response"] == wantOwner &&
			owners["turn"] == "main" && owners["session"] == "main" && owners["unrelated"] == "main" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Compact lineage durable owner projection: %+v lineage=%q errors=%v/%v", owners, lineage, err, lineageErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s %s: applied Ready profiles; exact full-history input; one WebSocket handshake/attempt on %s; unchanged Compact lineage and durable response owner\n", p.name, scenario, wantOwner)
	return nil
}

func verifyCompactLineage04383(wantOwner string, handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	wantAccount := map[string]string{"main": "account-a", "second": "account-b"}[wantOwner]
	if invalid || wantAccount == "" || !reflect.DeepEqual(handshakes, []string{wantAccount}) || len(attempts) != 1 ||
		!attempts[0].ready || attempts[0].owner != wantAccount || attempts[0].turn != "compact-turn" ||
		!sameTransportJSON04383(attempts[0].body, compactLineageMessage04383) ||
		!reflect.DeepEqual(frames, []string{transportDelta04383, transportCompleted04383}) {
		return fmt.Errorf("Compact lineage owner/provenance/frame mismatch: want=%s handshakes=%v attempts=%+v invalid=%t frames=%q", wantAccount, handshakes, attempts, invalid, frames)
	}
	return nil
}

func readCompactLineageOwner04383(p product) (string, error) {
	const alias = "__compact_turn_state__:compact-turn"
	if p.name == "prodex" {
		raw, err := os.ReadFile(filepath.Join(p.state, "runtime-continuations.json"))
		if err != nil {
			return "", err
		}
		var state struct {
			Value struct {
				Turns map[string]struct {
					Owner string `json:"profile_name"`
				} `json:"turn_state_bindings"`
			} `json:"value"`
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			return "", err
		}
		return state.Value.Turns[alias].Owner, nil
	}
	raw, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return "", err
	}
	var accounts struct {
		Accounts []struct{ ID, Name string }
	}
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return "", err
	}
	names := make(map[string]string, len(accounts.Accounts))
	for _, account := range accounts.Accounts {
		names[account.ID] = account.Name
	}
	raw, err = os.ReadFile(filepath.Join(p.state, "routing.json"))
	if err != nil {
		return "", err
	}
	var bindings struct {
		Bindings []struct {
			Key   string
			Owner string `json:"account_id"`
		}
	}
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte("turn:" + alias))
	for _, binding := range bindings.Bindings {
		if binding.Key == hex.EncodeToString(key[:]) {
			owner, valid := names[binding.Owner]
			if !valid {
				return "", errors.New("unregistered Compact lineage owner")
			}
			return owner, nil
		}
	}
	return "", nil
}
