package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpPreviousTextRequest04383 = `{"model":"gpt-6","previous_response_id":"resp-main","session_id":"safe-session","input":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"continue plain previous"}]}`
const httpPreviousTextReply04383 = `{"id":"resp-previous-text-fallback","object":"response","output":[]}`
const httpPreviousTextStale04383 = `{"error":{"code":"stale_continuation","message":"Upstream no longer recognizes this conversation chain before output started. Retry from the last user message or restart the Codex turn; Prodex will not send a fresh request without the missing context."}}`

func httpPreviousTextShapes04383() []string {
	return []string{"plain-code", "plain-message", "plain-invalid-id", "plain-tool-context", "valid-json-code", "unrelated", "invalid-surrogate", "invalid-overflow", "invalid-utf8"}
}

func httpPreviousTextBody04383(shape string) string {
	switch shape {
	case "plain-code":
		return "previous_response_not_found: response chain missing"
	case "plain-message":
		return "Previous response chain not found on the selected account"
	case "plain-invalid-id":
		return "Invalid `previous_response_id`."
	case "plain-tool-context":
		return "invalid_request_error: no tool call found for call_id"
	case "valid-json-code":
		return `{"error":{"code":"previous_response_not_found"}}`
	case "invalid-surrogate":
		return `{"error":{"code":"previous_response_not_found"},"unused":"\ud800"}`
	case "invalid-overflow":
		return `{"error":{"code":"previous_response_not_found"},"unused":1e309}`
	case "invalid-utf8":
		return `{"error":{"code":"previous_response_not_found"},"unused":"` + string([]byte{0xff}) + `"}`
	default:
		return "unrelated request failed"
	}
}

func httpPreviousTextRecognized04383(shape string) bool {
	return shape == "plain-code" || shape == "plain-message" || shape == "plain-invalid-id" || shape == "plain-tool-context" || shape == "valid-json-code"
}

func runHTTPPreviousTextProduct04383(root string, p product) error {
	for _, shape := range httpPreviousTextShapes04383() {
		if err := runHTTPPreviousTextCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("HTTP previous text %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPPreviousTextCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-previous-text-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-previous-text-" + shape)
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
			return fmt.Errorf("HTTP previous text warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(deadline) {
			return errors.New("HTTP previous-text fixture lacks two authenticated Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpPreviousTextRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), backend.attempts...), invalid: backend.invalid}
	backend.mu.Unlock()
	if err := verifyHTTPPreviousText04383(shape, result); err != nil {
		return err
	}
	// An owned previous response must never be re-sent on a second
	// profile. Tagged Prodex returns stale_continuation after the owner
	// confirms that the chain is unavailable; unrelated malformed 400
	// bodies are passed through unchanged.
	wantRecovered := ""
	until := time.Now().Add(3 * time.Second)
	for {
		owner, err := readHTTPResponseOwner04383(p, "resp-previous-text-fallback")
		oldSecond, oldErr := readHTTPResponseOwner04383(p, "resp-ready-b")
		original, originalErr := readHTTPResponseOwner04383(p, "resp-main")
		if err == nil && oldErr == nil && originalErr == nil && owner == wantRecovered && oldSecond == "second" && original == "main" {
			break
		}
		if time.Now().After(until) {
			return fmt.Errorf("HTTP previous text durable response owner=%q want=%q original=%q old second=%q errors=%v/%v/%v", owner, wantRecovered, original, oldSecond, err, originalErr, oldErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s previous text %s: authenticated two Ready owners; %d byte-exact attempts; HTTP %d; durable fallback=%q\n", p.name, shape, len(result.attempts), result.reply.status, wantRecovered)
	return nil
}

func verifyHTTPPreviousText04383(shape string, result httpSoftConflictResult04383) error {
	wantOwners := []string{"account-a"}
	status, body := http.StatusBadRequest, httpPreviousTextBody04383(shape)
	if httpPreviousTextRecognized04383(shape) {
		status, body = http.StatusConflict, httpPreviousTextStale04383
	}
	if result.invalid || result.reply.status != status || result.reply.body != body || len(result.attempts) != len(wantOwners) {
		return fmt.Errorf("HTTP previous text changed precommit outcome: shape=%s status=%d invalid=%t attempts=%+v reply=%q", shape, result.reply.status, result.invalid, result.attempts, result.reply.body)
	}
	for index, attempt := range result.attempts {
		if attempt.owner != wantOwners[index] || !attempt.ready || attempt.body != httpPreviousTextRequest04383 || attempt.turn != "" || attempt.metadata != "" || attempt.trace != "" {
			return fmt.Errorf("HTTP previous text wrong account/Ready/opaque request: shape=%s index=%d attempt=%+v", shape, index, attempt)
		}
	}
	return nil
}
