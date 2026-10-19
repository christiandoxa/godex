package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"time"
)

const compactReply04383 = `{"id":"resp-transport-recovered","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"synthetic-opaque"}]}`
const compactRetry04383 = `{"error":{"code":"previous_response_not_found","message":"Previous response was not found. Retrying the full request."}}`

type httpTransportResult04383 struct {
	reply    exchange
	attempts []transportAttempt04383
	owners   map[string]string
	invalid  bool
}

func runHTTPCompactProduct04383(root string, p product) error {
	for _, scenario := range []string{"turn-only", "with-session", "previous-response"} {
		if err := runHTTPTransportCase04383(filepath.Join(root, scenario), p, scenario, "/responses/compact"); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	return nil
}

func runHTTPTransportCase04383(root string, p product, scenario, route string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "full-history"); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newHTTPTransportBackend04383(route)
	defer b.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	client := &http.Client{Timeout: 10 * time.Second}
	for _, previous := range []string{"resp-main", "resp-ready-b"} {
		reply, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || reply.status != 200 || !strings.Contains(reply.body, "resp-warmup") {
			return fmt.Errorf("owner warmup %s: status=%d err=%v", previous, reply.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(until) {
			return errors.New("both Compact owners lack applied authenticated Ready evidence")
		}
		time.Sleep(10 * time.Millisecond)
	}
	owners, err := readTransportOwners04383(p)
	if err != nil || owners["turn"] != "main" || owners["session"] != "main" || owners["unrelated"] != "main" {
		return fmt.Errorf("Compact warmups changed the initial owner: owners=%v err=%v", owners, err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+route, strings.NewReader(httpTransportBody04383(scenario)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-codex-turn-state", "turn-a")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return err
	}
	stop()
	stop = nil
	owners, err = readTransportOwners04383(p)
	if err != nil {
		return err
	}
	b.mu.Lock()
	result := httpTransportResult04383{reply: exchange{status: response.StatusCode, body: string(payload)}, attempts: append([]transportAttempt04383(nil), b.attempts...), owners: owners, invalid: b.invalid}
	b.mu.Unlock()
	verify := verifyHTTPCompact04383
	if route == "/responses" {
		verify = verifyHTTPResponsesTurn04383
	}
	if err := verify(scenario, result); err != nil {
		return err
	}
	fmt.Printf("%s HTTP %s %s: applied Ready owners=2; upstream_requests=%d; status=%d; opaque wire and durable owners preserved\n", p.name, route, scenario, len(result.attempts), result.reply.status)
	return nil
}

func httpTransportBody04383(scenario string) string {
	body := strings.Replace(transportHistory04383, `"type":"response.create",`, "", 1)
	if scenario == "turn-only" {
		return strings.Replace(body, `"session_id":"sess-transport",`, "", 1)
	}
	if scenario == "previous-response" {
		return strings.Replace(body, `"model":"gpt-6",`, `"model":"gpt-6","previous_response_id":"resp-main",`, 1)
	}
	return body
}

func newHTTPTransportBackend04383(route string) *transportBackend04383 {
	b := &transportBackend04383{receipts: make(map[string]int)}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, route) || r.Header.Get("x-codex-turn-state") == "" {
			b.serve(w, r)
			return
		}
		owner := r.Header.Get("ChatGPT-Account-Id")
		expected := map[string]string{"account-a": "Bearer token-a", "account-b": "Bearer token-b"}[owner]
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		b.mu.Lock()
		b.invalid = b.invalid || err != nil || r.Method != http.MethodPost || expected == "" || r.Header.Get("Authorization") != expected
		b.attempts = append(b.attempts, transportAttempt04383{owner: owner, turn: r.Header.Get("x-codex-turn-state"), body: string(body), ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
		b.mu.Unlock()
		if owner == "account-a" {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		reply := compactReply04383
		if route == "/responses" {
			reply = `{"id":"resp-transport-recovered","object":"response","output":[]}`
		}
		writeJSON(w, 200, reply)
	}))
	return b
}

func verifyHTTPCompact04383(scenario string, result httpTransportResult04383) error {
	count, status, body := 2, http.StatusOK, compactReply04383
	session, unrelated := "main", "main"
	if scenario == "previous-response" {
		count, status, body, unrelated = 1, http.StatusBadRequest, compactRetry04383, ""
	} else if scenario == "with-session" {
		session = "__prodex_hard_binding_conflict__"
	} else if scenario != "turn-only" {
		return errors.New("unknown Compact transport scenario")
	}
	if result.invalid || len(result.attempts) != count || result.reply.status != status || result.reply.body != body {
		return fmt.Errorf("Compact auth, dispatch count or terminal response changed: invalid=%t attempts=%d status=%d body=%s", result.invalid, len(result.attempts), result.reply.status, result.reply.body)
	}
	for index, attempt := range result.attempts {
		wantOwner := "account-a"
		if index == 1 {
			wantOwner = "account-b"
		}
		if !attempt.ready || attempt.owner != wantOwner || attempt.turn != "turn-a" || attempt.body != httpTransportBody04383(scenario) {
			return fmt.Errorf("Compact attempt %d changed authenticated readiness, owner or opaque wire", index)
		}
	}
	if result.owners["turn"] != "" || result.owners["response"] != "" || result.owners["unrelated"] != unrelated || result.owners["session"] != session {
		return fmt.Errorf("Compact durable ownership changed: %v", result.owners)
	}
	return nil
}
