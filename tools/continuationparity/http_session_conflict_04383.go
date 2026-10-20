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
	"time"
)

const httpSoftConflictRequest04383 = `{"model":"gpt-6","session_id":"http-conflict","input":[{"role":"user","content":"independent complete request"}]}`
const httpSoftConflictResponse04383 = `{"id":"resp-http-soft-conflict","object":"response","output":[]}`
const httpSoftConflictOwner04383 = "__prodex_hard_binding_conflict__"

type httpSoftConflictResult04383 struct {
	reply                       exchange
	attempts                    []transportAttempt04383
	invalid                     bool
	sessionOwner, responseOwner string
}

func runHTTPSoftSessionConflictProduct04383(root string, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-session-conflict"); err != nil {
		return err
	}
	owner, err := readCompactBindingOwner04383(p, "session", "http-conflict")
	if err != nil || owner != httpSoftConflictOwner04383 {
		return fmt.Errorf("invalid verified HTTP conflict fixture: owner=%q err=%v", owner, err)
	}
	// The authenticated Ready gate additionally requires evidence that Prodex
	// applied both asynchronous quota receipts, not just backend 200s.
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("http-session-conflict")
	defer b.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	// Both profiles have applied authenticated Ready quota receipts, with main
	// warmed last; any fresh Ready choice is legal, but dispatch must occur.
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("HTTP soft-session conflict warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	readyDeadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(readyDeadline) {
			return errors.New("HTTP soft-session conflict lacks two applied Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpSoftConflictRequest04383)
	if err != nil {
		return fmt.Errorf("HTTP conflicted session request: %w", err)
	}
	b.mu.Lock()
	attempts := append([]transportAttempt04383(nil), b.attempts...)
	invalid := b.invalid
	b.mu.Unlock()
	result := httpSoftConflictResult04383{reply: reply, attempts: attempts, invalid: invalid}
	if err := verifyHTTPSoftConflict04383(result); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		result.sessionOwner, err = readCompactBindingOwner04383(p, "session", "http-conflict")
		if err == nil {
			result.responseOwner, err = readHTTPResponseOwner04383(p, "resp-http-soft-conflict")
		}
		if err == nil && result.sessionOwner == httpSoftConflictOwner04383 && result.responseOwner != "" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTP soft session durable mismatch: session=%q response=%q err=%v", result.sessionOwner, result.responseOwner, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if result.responseOwner != "main" && result.responseOwner != "second" {
		return fmt.Errorf("HTTP response owner not registered: %q", result.responseOwner)
	}
	if (result.responseOwner == "main" && result.attempts[0].owner != "account-a") ||
		(result.responseOwner == "second" && result.attempts[0].owner != "account-b") {
		return fmt.Errorf("HTTP response bound to wrong selected account: response=%q attempt=%q", result.responseOwner, result.attempts[0].owner)
	}
	fmt.Printf("%s HTTP soft session conflict: one Ready owner=%s; 200 exact output; direct conflict marker retained; durable response correctly bound\n", p.name, result.responseOwner)
	return nil
}

func verifyHTTPSoftConflict04383(r httpSoftConflictResult04383) error {
	if r.invalid || r.reply.status != http.StatusOK || r.reply.body != httpSoftConflictResponse04383 || len(r.attempts) != 1 ||
		!r.attempts[0].ready || (r.attempts[0].owner != "account-a" && r.attempts[0].owner != "account-b") ||
		r.attempts[0].turn != "" || r.attempts[0].body != httpSoftConflictRequest04383 {
		return fmt.Errorf("HTTP soft session rejected, replayed, or changed wire: invalid=%t status=%d body=%s attempts=%+v", r.invalid, r.reply.status, r.reply.body, r.attempts)
	}
	return nil
}

func readHTTPResponseOwner04383(p product, responseID string) (string, error) {
	if p.name == "prodex" {
		raw, err := os.ReadFile(filepath.Join(p.state, "runtime-continuations.json"))
		if err != nil {
			return "", err
		}
		var state struct {
			Value struct {
				Responses map[string]struct {
					Owner string `json:"profile_name"`
				} `json:"response_profile_bindings"`
			} `json:"value"`
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			return "", err
		}
		return state.Value.Responses[responseID].Owner, nil
	}
	raw, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return "", err
	}
	var state struct{ Accounts []struct{ ID, Name string } }
	if err := json.Unmarshal(raw, &state); err != nil {
		return "", err
	}
	names := make(map[string]string, len(state.Accounts))
	for _, a := range state.Accounts {
		names[a.ID] = a.Name
	}
	raw, err = os.ReadFile(filepath.Join(p.state, "routing.json"))
	if err != nil {
		return "", err
	}
	var routing struct {
		Bindings []struct {
			Key       string
			AccountID string `json:"account_id"`
		}
	}
	if err := json.Unmarshal(raw, &routing); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte("previous:" + responseID))
	for _, binding := range routing.Bindings {
		if binding.Key == hex.EncodeToString(digest[:]) {
			return names[binding.AccountID], nil
		}
	}
	return "", nil
}
