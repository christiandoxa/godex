package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const httpResponseSessionID04383 = "resp-response-session"
const httpResponseSessionName04383 = "payload-only-session"

var httpResponseSessionShapes04383 = []string{
	"body-session", "nested-session", "data-session", "conversation-alias",
	"thread-alias", "response-header", "request-session",
	"triple-ids", "duplicate-ids", "legacy-and-root", "ignore-event-root",
	"turn-sorted-map", "turn-unicode-only", "turn-padded-only", "turn-array-order",
	"turn-nested-priority", "turn-array-objects",
	"invalid-surrogate-high", "invalid-surrogate-low", "invalid-surrogate-key", "valid-surrogate-pair",
}

func httpResponseSessionRequest04383(shape string) string {
	if shape == "request-session" {
		return `{"model":"gpt-6","session_id":"payload-only-session","input":[{"role":"user","content":"explicit session"}]}`
	}
	return `{"model":"gpt-6","input":[{"role":"user","content":"independent response"}]}`
}

func httpResponseSessionReply04383(shape string) string {
	switch shape {
	case "body-session":
		return `{"id":"resp-response-session","object":"response","session_id":"payload-only-session","output":[]}`
	case "nested-session":
		return `{"id":"resp-response-session","object":"response","response":{"session_id":"payload-only-session"},"output":[]}`
	case "data-session":
		return `{"id":"resp-response-session","object":"response","data":{"session_id":"payload-only-session"},"output":[]}`
	case "conversation-alias":
		return `{"id":"resp-response-session","object":"response","conversation_id":"payload-only-session","output":[]}`
	case "thread-alias":
		return `{"id":"resp-response-session","object":"response","thread_id":"payload-only-session","output":[]}`
	case "invalid-surrogate-high":
		return `{"id":"resp-response-session","object":"response","note":"\ud800","output":[]}`
	case "invalid-surrogate-low":
		return `{"id":"resp-response-session","object":"response","note":"\udc00","output":[]}`
	case "invalid-surrogate-key":
		return `{"id":"resp-response-session","object":"response","note\ud800":"x","output":[]}`
	case "valid-surrogate-pair":
		return `{"id":"resp-response-session","object":"response","note":"\ud83d\ude00","output":[]}`
	case "triple-ids":
		return `{"response":{"id":"resp-response-session"},"response_id":"resp-alias-middle","object":"response","id":"resp-alias-outer","output":[]}`
	case "duplicate-ids":
		return `{"response":{"id":"resp-response-session"},"response_id":"resp-response-session","object":"response","id":"resp-alias-outer","output":[]}`
	case "legacy-and-root":
		return `{"response_id":"resp-response-session","object":"response","id":"resp-alias-outer","output":[]}`
	case "ignore-event-root":
		return `{"response_id":"resp-response-session","object":"event","id":"event-unrelated","output":[]}`
	case "turn-sorted-map":
		return `{"id":"resp-response-session","object":"response","headers":{"x-codex-turn-state":"lower-turn","X-CODEX-TURN-STATE":"upper-turn"},"output":[]}`
	case "turn-unicode-only":
		return `{"id":"resp-response-session","object":"response","headers":{"x-codex-turn-ſtate":"impostor-turn"},"output":[]}`
	case "turn-padded-only":
		return `{"id":"resp-response-session","object":"response","headers":{" x-codex-turn-state ":"impostor-turn"},"output":[]}`
	case "turn-array-order":
		return `{"id":"resp-response-session","object":"response","headers":[["x-codex-turn-state","lower-turn"],["X-CODEX-TURN-STATE","upper-turn"]],"output":[]}`
	case "turn-nested-priority":
		return `{"id":"resp-response-session","object":"response","response":{"headers":{"X-CODEX-TURN-STATE":"nested-turn"}},"headers":{"x-codex-turn-state":"root-turn"},"output":[]}`
	case "turn-array-objects":
		return `{"id":"resp-response-session","object":"response","headers":[{"name":"x-codex-turn-ſtate","value":"impostor-turn"},{"key":"X-CODEX-TURN-STATE","values":["  ","real-turn"]}],"output":[]}`
	default:
		return `{"id":"resp-response-session","object":"response","output":[]}`
	}
}

func httpResponseSessionHeader04383(shape string) string {
	if shape == "response-header" {
		return httpResponseSessionName04383
	}
	return ""
}

func runHTTPResponseSessionProduct04383(root string, p product) error {
	for _, shape := range httpResponseSessionShapes04383 {
		if err := runHTTPResponseSessionCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("response session %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPResponseSessionCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-response-session-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-response-session-" + shape)
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
			return fmt.Errorf("response-session warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("response-session fixture has no authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpResponseSessionRequest04383(shape))
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, invalid: backend.invalid,
		attempts: append([]transportAttempt04383(nil), backend.attempts...)}
	backend.mu.Unlock()
	if err := verifyHTTPResponseSession04383(shape, result); err != nil {
		return err
	}
	wantSession := ""
	if shape == "request-session" {
		wantSession = "main"
	}
	wantResponse := "main"
	if strings.HasPrefix(shape, "invalid-surrogate-") {
		wantResponse = ""
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		responseOwner, responseErr := readHTTPResponseOwner04383(p, httpResponseSessionID04383)
		sessionOwner, sessionErr := readCompactBindingOwner04383(p, "session", httpResponseSessionName04383)
		if responseErr == nil && sessionErr == nil && responseOwner == wantResponse && sessionOwner == wantSession &&
			httpResponseIDAliasesMatch04383(p, shape) && httpResponseTurnHeadersMatch04383(p, shape) {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("response/session ownership mismatch: response=%q want_response=%q session=%q want_session=%q errors=%v/%v", responseOwner, wantResponse, sessionOwner, wantSession, responseErr, sessionErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s response %s: one Ready main attempt, exact response, response owner %q, session owner %q\n", p.name, shape, wantResponse, wantSession)
	return nil
}

func verifyHTTPResponseSession04383(shape string, result httpSoftConflictResult04383) error {
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpResponseSessionReply04383(shape) ||
		len(result.attempts) != 1 || !result.attempts[0].ready || result.attempts[0].owner != "account-a" ||
		result.attempts[0].body != httpResponseSessionRequest04383(shape) || result.attempts[0].turn != "" ||
		result.attempts[0].metadata != "" || result.attempts[0].trace != "" {
		return fmt.Errorf("response-only session fixture changed wire/owner: shape=%s invalid=%t status=%d attempts=%+v", shape, result.invalid, result.reply.status, result.attempts)
	}
	return nil
}

// A tagged Responses payload may present three distinct response IDs. The
// IDs are independently usable for later continuation, not just display IDs.
func httpResponseIDAliasesMatch04383(p product, shape string) bool {
	var extraIDs []string
	switch shape {
	case "triple-ids":
		extraIDs = []string{"resp-alias-middle", "resp-alias-outer"}
	case "duplicate-ids", "legacy-and-root":
		extraIDs = []string{"resp-alias-outer"}
	case "ignore-event-root":
		owner, err := readHTTPResponseOwner04383(p, "event-unrelated")
		return err == nil && owner == ""
	}
	for _, id := range extraIDs {
		owner, err := readHTTPResponseOwner04383(p, id)
		if err != nil || owner != "main" {
			return false
		}
	}
	return true
}

// The metadata header walker is strictly ASCII and follows serde_json map
// ordering, while an explicit JSON array retains its transmitted order.
func httpResponseTurnHeadersMatch04383(p product, shape string) bool {
	var expected string
	switch shape {
	case "turn-sorted-map":
		expected = "upper-turn"
	case "turn-array-order":
		expected = "lower-turn"
	case "turn-nested-priority":
		expected = "nested-turn"
	case "turn-array-objects":
		expected = "real-turn"
	case "turn-unicode-only", "turn-padded-only":
		// No supplied header is a valid ASCII x-codex-turn-state key.
	default:
		return true
	}
	for _, turn := range []string{"upper-turn", "lower-turn", "nested-turn", "root-turn", "real-turn", "impostor-turn"} {
		wantOwner := ""
		if turn == expected {
			wantOwner = "main"
		}
		owner, err := readCompactBindingOwner04383(p, "turn", turn)
		if err != nil || owner != wantOwner {
			return false
		}
	}
	return true
}
