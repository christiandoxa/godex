package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpShadowedTurnReply04383 = `{"id":"resp-http-turn-shadow","object":"response","output":[]}`

func httpShadowedTurnBody04383(shape string) string {
	field := `"x-codex-turn-state":null,`
	switch shape {
	case "blank":
		field = `"x-codex-turn-state":"  ",`
	case "missing":
		field = ""
	}
	return `{"model":"gpt-6",` + field + `"client_metadata":{"x-codex-turn-state":"shadow-turn"},"input":[{"role":"user","content":"independent complete request"}]}`
}

func runHTTPShadowedTurnProduct04383(root string, p product) error {
	for _, shape := range []string{"null", "blank", "missing", "header"} {
		if err := runHTTPShadowedTurnCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("x-codex-turn-state %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPShadowedTurnCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "turn-http-shadow-"+shape); err != nil {
		return err
	}
	initial, err := readCompactBindingOwner04383(p, "turn", "shadow-turn")
	if err != nil || initial != "second" {
		return fmt.Errorf("HTTP shadow fixture missing verified nested turn owner: %q err=%v", initial, err)
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("turn-http-shadow-" + shape)
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
			return fmt.Errorf("HTTP shadow warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(until) {
			return errors.New("HTTP turn shadow fixture lacks authenticated Ready evidence on both profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	body := httpShadowedTurnBody04383(shape)
	var reply exchange
	if shape == "header" {
		reply, err = postContextHeader(client, endpoint+"/responses", body, "x-codex-turn-state", "shadow-turn")
	} else {
		reply, err = post(client, endpoint+"/responses", body)
	}
	if err != nil {
		return err
	}
	b.mu.Lock()
	r := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPShadowedTurn04383(shape, r); err != nil {
		return err
	}
	wantOwner := "main"
	if shape == "header" {
		wantOwner = "second"
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		turnOwner, turnErr := readCompactBindingOwner04383(p, "turn", "shadow-turn")
		responseOwner, responseErr := readHTTPResponseOwner04383(p, "resp-http-turn-shadow")
		if turnErr == nil && responseErr == nil && turnOwner == "second" && responseOwner == wantOwner {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTP turn shadow durable mismatch: turn=%q response=%q want=%q errors=%v/%v", turnOwner, responseOwner, wantOwner, turnErr, responseErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP turn direct=%s: two Ready profiles; one %s attempt; exact body; HTTP 200; nested owner unchanged\n", p.name, shape, wantOwner)
	return nil
}

func verifyHTTPShadowedTurn04383(shape string, result httpSoftConflictResult04383) error {
	wantOwner := "account-a"
	if shape == "header" {
		wantOwner = "account-b"
	}
	wantTurn := ""
	if shape == "header" {
		wantTurn = "shadow-turn"
	}
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpShadowedTurnReply04383 ||
		len(result.attempts) != 1 || !result.attempts[0].ready || result.attempts[0].owner != wantOwner ||
		result.attempts[0].turn != wantTurn || result.attempts[0].body != httpShadowedTurnBody04383(shape) {
		return fmt.Errorf("HTTP turn shadow changed owner/wire: shape=%s invalid=%t status=%d attempts=%+v", shape, result.invalid, result.reply.status, result.attempts)
	}
	return nil
}
