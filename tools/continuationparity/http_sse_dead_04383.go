package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

const httpSSEDeadRequest04383 = `{"model":"gpt-6","previous_response_id":"resp-main","stream":true,"input":[{"role":"user","content":"committed previous chain fails"}]}`
const httpSSEDeadPrelude04383 = "data: " + `{"type":"response.output_text.delta","delta":"started","response":{"id":"resp-dead-output","turn_state":"state-dead"}}` + "\n\n"
const httpSSEDeadFailure04383 = "data: " + `{"type":"response.failed","response":{"id":"resp-dead-terminal","error":{"code":"previous_response_not_found"}}}` + "\n\n"
const httpSSEDeadWire04383 = httpSSEDeadPrelude04383 + httpSSEDeadFailure04383

func runHTTPSSEDeadProduct04383(root string, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-sse-dead-chain"); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-sse-dead-chain")
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
			return fmt.Errorf("SSE dead chain warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("dead-chain fixture lacks two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpSSEDeadRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	attempts := append([]transportAttempt04383(nil), backend.attempts...)
	invalid := backend.invalid
	backend.mu.Unlock()
	if err := verifyHTTPSSEDead04383(reply, attempts, invalid); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		old, err1 := readHTTPResponseOwner04383(p, "resp-main")
		prefix, err2 := readHTTPResponseOwner04383(p, "resp-dead-output")
		failed, err3 := readHTTPResponseOwner04383(p, "resp-dead-terminal")
		other, err4 := readHTTPResponseOwner04383(p, "resp-ready-b")
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil && old == "" && prefix == "" && failed == "" && other == "second" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("SSE dead chain durable mismatch: original=%q prefix=%q failed=%q unaffected=%q errors=%v/%v/%v/%v", old, prefix, failed, other, err1, err2, err3, err4)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s SSE postcommit dead chain: authenticated Ready main; one exact inference; HTTP 200 full stream; three dead response owners and unchanged second\n", p.name)
	return nil
}

func verifyHTTPSSEDead04383(reply exchange, attempts []transportAttempt04383, invalid bool) error {
	if invalid || reply.status != http.StatusOK || reply.body != httpSSEDeadWire04383 || len(attempts) != 1 ||
		!attempts[0].ready || attempts[0].owner != "account-a" || attempts[0].body != httpSSEDeadRequest04383 ||
		attempts[0].turn != "" || attempts[0].metadata != "" || attempts[0].trace != "" {
		return fmt.Errorf("SSE dead chain changed owner, wire or auth: status=%d invalid=%t body=%q attempts=%+v", reply.status, invalid, reply.body, attempts)
	}
	return nil
}
