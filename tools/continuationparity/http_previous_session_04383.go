package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const httpPreviousSessionRequest04383 = `{"model":"gpt-6","previous_response_id":"resp-main","session_id":"http-hard-session","input":[{"role":"user","content":"independent previous response continuation"}]}`
const httpTurnSessionRequest04383 = `{"model":"gpt-6","x-codex-turn-state":"turn-a","session_id":"http-hard-session","input":[{"role":"user","content":"independent previous response continuation"}]}`
const httpPreviousSessionResponse04383 = `{"id":"resp-http-hard-previous","object":"response","output":[]}`

func runHTTPPreviousSessionProduct04383(root string, p product) error {
	for _, kind := range []string{"previous", "turn"} {
		for _, foreign := range []bool{false, true} {
			scenario := "http-" + kind + "-session-new"
			if foreign {
				scenario = "http-" + kind + "-session-foreign"
			}
			if err := runHTTPPreviousSessionCase04383(filepath.Join(root, scenario), p, scenario, foreign); err != nil {
				return fmt.Errorf("%s: %w", scenario, err)
			}
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
	isTurn := strings.HasPrefix(scenario, "http-turn-session-")
	body := httpPreviousSessionRequest04383
	if isTurn {
		body = httpTurnSessionRequest04383
	}
	reply, err := postHTTPHardSession04383(client, endpoint+"/responses", body, isTurn)
	if err != nil {
		return fmt.Errorf("hard %s+session request: %w", scenario, err)
	}
	b.mu.Lock()
	r := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPPreviousSession04383(r, isTurn); err != nil {
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
	fmt.Printf("%s %s: verified hard owner main; one Ready upstream; HTTP 200; durable direct session=%s and response=main\n", p.name, scenario, want)
	return nil
}

func verifyHTTPPreviousSession04383(r httpSoftConflictResult04383, isTurn bool) error {
	wantBody, wantTurn := httpPreviousSessionRequest04383, ""
	if isTurn {
		wantBody, wantTurn = httpTurnSessionRequest04383, "turn-a"
	}
	if r.invalid || r.reply.status != http.StatusOK || r.reply.body != httpPreviousSessionResponse04383 ||
		len(r.attempts) != 1 || r.attempts[0].owner != "account-a" || !r.attempts[0].ready ||
		r.attempts[0].turn != wantTurn || r.attempts[0].body != wantBody {
		return fmt.Errorf("HTTP hard previous owner/session/response mismatch: invalid=%t status=%d reply=%s attempts=%+v", r.invalid, r.reply.status, r.reply.body, r.attempts)
	}
	return nil
}

// The reference adapter accepts both the encoded body member and explicit
// outbound turn-state header; use a real HTTP request to compare both carriers.
func postHTTPHardSession04383(client *http.Client, url, body string, withTurn bool) (exchange, error) {
	if !withTurn {
		return post(client, url, body)
	}
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return exchange{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-codex-turn-state", "turn-a")
	response, err := client.Do(request)
	if err != nil {
		return exchange{}, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return exchange{status: response.StatusCode, body: string(payload)}, err
}
