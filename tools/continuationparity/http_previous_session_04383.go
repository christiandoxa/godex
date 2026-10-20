package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpPreviousSessionRequest04383 = `{"model":"gpt-6","previous_response_id":"resp-main","session_id":"http-hard-session","input":[{"role":"user","content":"independent previous response continuation"}]}`
const httpPreviousSessionResponse04383 = `{"id":"resp-http-hard-previous","object":"response","output":[]}`

func runHTTPPreviousSessionProduct04383(root string, p product) error {
	for _, foreign := range []bool{false, true} {
		scenario := "http-previous-session-new"
		if foreign {
			scenario = "http-previous-session-foreign"
		}
		if err := runHTTPPreviousSessionCase04383(filepath.Join(root, scenario), p, scenario, foreign); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	return nil
}

func runHTTPPreviousSessionCase04383(root string, p product, scenario string, foreign bool) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, scenario); err != nil {
		return err
	}
	initial, err := readCompactBindingOwner04383(p, "session", "http-hard-session")
	wantInitial := ""
	if foreign {
		wantInitial = "second"
	}
	if err != nil || initial != wantInitial {
		return fmt.Errorf("invalid hard-previous session fixture: owner=%q want=%q err=%v", initial, wantInitial, err)
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383(scenario)
	defer b.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("previous/session warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(until) {
			return errors.New("previous/session fixture missing applied authenticated Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpPreviousSessionRequest04383)
	if err != nil {
		return fmt.Errorf("hard previous+session request: %w", err)
	}
	b.mu.Lock()
	r := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPPreviousSession04383(r); err != nil {
		return err
	}
	want := "main"
	if foreign {
		want = httpSoftConflictOwner04383
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		r.sessionOwner, err = readCompactBindingOwner04383(p, "session", "http-hard-session")
		if err == nil {
			r.responseOwner, err = readHTTPResponseOwner04383(p, "resp-http-hard-previous")
		}
		if err == nil && r.sessionOwner == want && r.responseOwner == "main" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("hard previous session durability changed: owner=%q want=%q response=%q err=%v", r.sessionOwner, want, r.responseOwner, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s %s: verified previous owner main; one Ready upstream; HTTP 200; durable direct session=%s and response=main\n", p.name, scenario, want)
	return nil
}

func verifyHTTPPreviousSession04383(r httpSoftConflictResult04383) error {
	if r.invalid || r.reply.status != http.StatusOK || r.reply.body != httpPreviousSessionResponse04383 ||
		len(r.attempts) != 1 || r.attempts[0].owner != "account-a" || !r.attempts[0].ready ||
		r.attempts[0].turn != "" || r.attempts[0].body != httpPreviousSessionRequest04383 {
		return fmt.Errorf("HTTP hard previous owner/session/response mismatch: invalid=%t status=%d reply=%s attempts=%+v", r.invalid, r.reply.status, r.reply.body, r.attempts)
	}
	return nil
}
