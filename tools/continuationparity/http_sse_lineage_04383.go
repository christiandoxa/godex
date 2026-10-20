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
	"strconv"
	"strings"
	"time"
)

const httpSSETurnLineageRequest04383 = `{"model":"gpt-6","previous_response_id":"resp-main","stream":true,"input":[{"role":"user","content":"separate turn states"}]}`
const httpSSETurnLineageWire04383 = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\",\"response\":{\"id\":\"resp-sse-one\",\"turn_state\":\"state-one\"}}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-sse-two\",\"turn_state\":\"state-two\"}}\n\n"

// The same strict per-response stream lineage is exercised when a later
// response omits turn-state (inherit), and when the first turn only becomes
// available after the precommit output was already committed (backfill).
func httpSSETurnLineageWireFor04383(shape string) string {
	first := `{"type":"response.output_text.delta","delta":"first","response":{"id":"resp-sse-one","turn_state":"state-one"}}`
	second := `{"type":"response.completed","response":{"id":"resp-sse-two","turn_state":"state-two"}}`
	switch shape {
	case "inherit":
		second = `{"type":"response.completed","response":{"id":"resp-sse-two"}}`
	case "late":
		first = `{"type":"response.output_text.delta","delta":"first","response":{"id":"resp-sse-one"}}`
	case "large-committed":
		second = `{"type":"response.completed","response":{"id":"resp-sse-two"},"padding":"` + strings.Repeat("x", 72<<10) + `"}`
	}
	return "data: " + first + "\n\n" + "data: " + second + "\n\n"
}

func httpSSETurnLineageExpected04383(shape, responseID string) string {
	if shape == "inherit" || shape == "partial" || shape == "large-committed" {
		return "state-one"
	}
	if shape == "late" {
		return "state-two"
	}
	if responseID == "resp-sse-one" {
		return "state-one"
	}
	return "state-two"
}

func runHTTPSSETurnLineageProduct04383(root string, p product) error {
	for _, shape := range []string{"distinct", "inherit", "late", "large-committed"} {
		if err := runHTTPSSETurnLineageCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("SSE lineage %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPSSETurnLineageCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-sse-turn-lineage-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-sse-turn-lineage-" + shape)
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("SSE lineage warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("SSE lineage fixture lacks two applied authenticated Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpSSETurnLineageRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, invalid: backend.invalid, attempts: append([]transportAttempt04383(nil), backend.attempts...)}
	backend.mu.Unlock()
	if err := verifyHTTPSSETurnLineageFor04383(shape, result); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		first, err1 := readHTTPResponseOwner04383(p, "resp-sse-one")
		second, err2 := readHTTPResponseOwner04383(p, "resp-sse-two")
		verified, err3 := httpSSETurnLineageOwnersFor04383(p, shape)
		if err1 == nil && err2 == nil && err3 == nil && first == "main" && second == "main" && verified {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("SSE turn lineage durable mismatch: first=%q second=%q verified=%t errors=%v/%v/%v", first, second, verified, err1, err2, err3)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s SSE turn lineage %s: Ready main, exact two-event stream, one upstream attempt; per-response durable turn-state associations\n", p.name, shape)
	return nil
}

func verifyHTTPSSETurnLineage04383(result httpSoftConflictResult04383) error {
	return verifyHTTPSSETurnLineageFor04383("distinct", result)
}

func verifyHTTPSSETurnLineageFor04383(shape string, result httpSoftConflictResult04383) error {
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpSSETurnLineageWireFor04383(shape) ||
		len(result.attempts) != 1 || !result.attempts[0].ready || result.attempts[0].owner != "account-a" ||
		result.attempts[0].body != httpSSETurnLineageRequest04383 || result.attempts[0].turn != "" ||
		result.attempts[0].metadata != "" || result.attempts[0].trace != "" {
		return fmt.Errorf("SSE turn lineage changed owner or stream/wire evidence: %+v", result)
	}
	return nil
}

func httpSSETurnLineageOwnersFor04383(p product, shape string) (bool, error) {
	if p.name == "prodex" {
		raw, err := os.ReadFile(filepath.Join(p.state, "runtime-continuations.json"))
		if err != nil {
			return false, err
		}
		var state struct {
			Value struct {
				Responses map[string]struct {
					Profile string `json:"profile_name"`
				} `json:"response_profile_bindings"`
			} `json:"value"`
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			return false, err
		}
		for _, tc := range []struct {
			id, turn string
			exists   bool
		}{
			{"resp-sse-one", "state-one", httpSSETurnLineageExpected04383(shape, "resp-sse-one") == "state-one"},
			{"resp-sse-one", "state-two", httpSSETurnLineageExpected04383(shape, "resp-sse-one") == "state-two"},
			{"resp-sse-two", "state-one", httpSSETurnLineageExpected04383(shape, "resp-sse-two") == "state-one"},
			{"resp-sse-two", "state-two", httpSSETurnLineageExpected04383(shape, "resp-sse-two") == "state-two"},
		} {
			key := "__response_turn_state__:" + strconv.Itoa(len(tc.id)) + ":" + tc.id + ":" + tc.turn
			profile := state.Value.Responses[key].Profile
			if (tc.exists && profile != "main") || (!tc.exists && profile != "") {
				return false, nil
			}
		}
		return true, nil
	}
	// Godex stores per-response opaque turn tokens in the owning account's
	// private CODEX_HOME. These sidecars are not stored in shared routing.json.
	raw, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return false, err
	}
	var state struct{ Accounts []struct{ ID, Name string } }
	if err := json.Unmarshal(raw, &state); err != nil {
		return false, err
	}
	mainID := ""
	for _, account := range state.Accounts {
		if account.Name == "main" {
			mainID = account.ID
		}
	}
	if mainID == "" {
		return false, errors.New("SSE lineage synthetic main profile missing")
	}
	for _, tc := range []struct{ id, turn string }{
		{"resp-sse-one", httpSSETurnLineageExpected04383(shape, "resp-sse-one")},
		{"resp-sse-two", httpSSETurnLineageExpected04383(shape, "resp-sse-two")},
	} {
		key := sha256.Sum256([]byte("previous:" + tc.id))
		path := filepath.Join(p.state, "accounts", mainID, "codex", ".godex-turn-state", hex.EncodeToString(key[:])+".json")
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var persisted struct {
			TurnState string `json:"turn_state"`
		}
		if err := json.Unmarshal(raw, &persisted); err != nil {
			return false, err
		}
		if persisted.TurnState != tc.turn {
			return false, nil
		}
	}
	return true, nil
}
