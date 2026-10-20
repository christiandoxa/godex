package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpResponseSessionID04383 = "resp-response-session"
const httpResponseSessionName04383 = "payload-only-session"

var httpResponseSessionShapes04383 = []string{
	"body-session", "nested-session", "data-session", "conversation-alias",
	"thread-alias", "response-header", "request-session",
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
	deadline := time.Now().Add(3 * time.Second)
	for {
		responseOwner, responseErr := readHTTPResponseOwner04383(p, httpResponseSessionID04383)
		sessionOwner, sessionErr := readCompactBindingOwner04383(p, "session", httpResponseSessionName04383)
		if responseErr == nil && sessionErr == nil && responseOwner == "main" && sessionOwner == wantSession {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("response/session ownership mismatch: response=%q session=%q want_session=%q errors=%v/%v", responseOwner, sessionOwner, wantSession, responseErr, sessionErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s response %s: one Ready main attempt, unchanged response, response owner main, session owner %q\n", p.name, shape, wantSession)
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
