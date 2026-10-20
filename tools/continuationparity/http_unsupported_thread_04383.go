package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

type httpUnsupportedThreadStep04383 struct {
	name, body, trace, owner, responseID string
}

var httpUnsupportedThreadSteps04383 = []httpUnsupportedThreadStep04383{
	{
		name: "producer", body: `{"model":"gpt-6","previous_response_id":"resp-ready-b","input":[{"role":"user","content":"thread producer"}]}`,
		trace: "trace-only", owner: "account-b", responseID: "resp-thread-producer",
	},
	{
		name: "restore", body: `{"model":"gpt-6","previous_response_id":"resp-main","input":[{"role":"user","content":"thread restore"}]}`,
		trace: "", owner: "account-a", responseID: "resp-thread-restore",
	},
	{
		name: "independent", body: `{"model":"gpt-6","input":[{"role":"user","content":"thread independent"}]}`,
		trace: "trace-only", owner: "account-a", responseID: "resp-thread-independent",
	},
}

func runHTTPUnsupportedThreadProduct04383(root string, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-unsupported-thread"); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("http-unsupported-thread")
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
			return fmt.Errorf("unsupported thread warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(until) {
			return errors.New("unsupported thread fixture has no two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, step := range httpUnsupportedThreadSteps04383 {
		var reply exchange
		if step.trace != "" {
			reply, err = postContextHeader(client, endpoint+"/responses", step.body, "x-codex-thread-id", step.trace)
		} else {
			reply, err = post(client, endpoint+"/responses", step.body)
		}
		if err != nil || reply.status != http.StatusOK || reply.body != httpUnsupportedThreadReply04383(step.responseID) {
			return fmt.Errorf("unsupported thread %s: reply=%+v err=%v", step.name, reply, err)
		}
	}
	b.mu.Lock()
	result := httpSoftConflictResult04383{reply: exchange{status: http.StatusOK, body: httpUnsupportedThreadReply04383("resp-thread-independent")},
		attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPUnsupportedThread04383(result); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		match := true
		for _, step := range httpUnsupportedThreadSteps04383 {
			owner, err := readHTTPResponseOwner04383(p, step.responseID)
			want := "main"
			if step.owner == "account-b" {
				want = "second"
			}
			if err != nil || owner != want {
				match = false
				break
			}
		}
		if match {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("unsupported thread trace changed durable HTTP response owners")
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP unsupported x-codex-thread-id: two Ready profiles; owner sequence second/main/main; exact wire; three durable response owners\n", p.name)
	return nil
}

func httpUnsupportedThreadReply04383(id string) string {
	return fmt.Sprintf(`{"id":%q,"object":"response","output":[]}`, id)
}

func verifyHTTPUnsupportedThread04383(r httpSoftConflictResult04383) error {
	if r.invalid || r.reply.status != http.StatusOK || r.reply.body != httpUnsupportedThreadReply04383("resp-thread-independent") ||
		len(r.attempts) != len(httpUnsupportedThreadSteps04383) {
		return fmt.Errorf("unsupported thread trace malformed attempts=%+v invalid=%t reply=%+v", r.attempts, r.invalid, r.reply)
	}
	for index, step := range httpUnsupportedThreadSteps04383 {
		got := r.attempts[index]
		if !got.ready || got.owner != step.owner || got.body != step.body || got.trace != step.trace || got.turn != "" || got.metadata != "" {
			return fmt.Errorf("unsupported thread %s mismatched upstream attempt: %+v", step.name, got)
		}
	}
	return nil
}
