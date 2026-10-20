package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpShadowedSessionReply04383 = `{"id":"resp-http-shadow","object":"response","output":[]}`

func httpShadowedSessionBody04383(shape string) string {
	// Request aliases are untrusted payload, unlike the one canonical
	// session_id and the documented client_metadata.session_id fallback.
	prefix := `{"model":"gpt-6",`
	suffix := `"input":[{"role":"user","content":"independent complete request"}]}`
	switch shape {
	case "metadata-nested", "metadata-direct", "metadata-null", "metadata-blank":
		return prefix + suffix
	case "metadata-fallback":
		return prefix + `"session_id":"shadow-session",` + suffix
	case "conversation":
		return prefix + `"conversation_id":"shadow-session",` + suffix
	case "thread":
		return prefix + `"thread_id":"shadow-session",` + suffix
	case "both":
		return prefix + `"conversation_id":"shadow-session","thread_id":"shadow-session",` + suffix
	case "direct":
		return prefix + `"session_id":"shadow-session",` + suffix
	}
	field := `"session_id":null,`
	switch shape {
	case "blank":
		field = `"session_id":"  ",`
	case "missing":
		field = ""
	}
	return `{"model":"gpt-6",` + field + `"client_metadata":{"session_id":"shadow-session"},"input":[{"role":"user","content":"independent complete request"}]}`
}

func httpShadowedSessionHeaderMetadata04383(shape string) string {
	switch shape {
	case "metadata-nested":
		return `{"client_metadata":{"session_id":"shadow-session"}}`
	case "metadata-direct":
		return `{"session_id":"shadow-session"}`
	case "metadata-null":
		return `{"session_id":null,"client_metadata":{"session_id":"shadow-session"}}`
	case "metadata-blank":
		return `{"session_id":"  ","client_metadata":{"session_id":"shadow-session"}}`
	case "metadata-fallback":
		return `{"session_id":null}`
	default:
		return ""
	}
}

func httpShadowedSessionExpectSecond04383(shape string) bool {
	switch shape {
	case "missing", "direct", "metadata-nested", "metadata-direct", "metadata-fallback":
		return true
	default:
		return false
	}
}

func runHTTPShadowedSessionProduct04383(root string, p product) error {
	for _, shape := range []string{"null", "blank", "missing", "conversation", "thread", "both", "direct", "metadata-nested", "metadata-direct", "metadata-null", "metadata-blank", "metadata-fallback"} {
		if err := runHTTPShadowedSessionCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("session_id %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPShadowedSessionCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "session-http-shadow-"+shape); err != nil {
		return err
	}
	initial, err := readCompactBindingOwner04383(p, "session", "shadow-session")
	if err != nil || initial != "second" {
		return fmt.Errorf("HTTP shadow fixture missing verified nested owner: %q err=%v", initial, err)
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("session-http-shadow-" + shape)
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
			return errors.New("HTTP shadow fixture lacks authenticated Ready evidence on both profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	body := httpShadowedSessionBody04383(shape)
	var reply exchange
	if header := httpShadowedSessionHeaderMetadata04383(shape); header != "" {
		reply, err = postContextHeader(client, endpoint+"/responses", body, "x-codex-turn-metadata", header)
	} else {
		reply, err = post(client, endpoint+"/responses", body)
	}
	if err != nil {
		return err
	}
	b.mu.Lock()
	r := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPShadowedSession04383(shape, r); err != nil {
		return err
	}
	wantOwner := "main"
	if httpShadowedSessionExpectSecond04383(shape) {
		wantOwner = "second"
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		sessionOwner, sessionErr := readCompactBindingOwner04383(p, "session", "shadow-session")
		responseOwner, responseErr := readHTTPResponseOwner04383(p, "resp-http-shadow")
		if sessionErr == nil && responseErr == nil && sessionOwner == "second" && responseOwner == wantOwner {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTP shadow durable mismatch: session=%q response=%q want=%q errors=%v/%v", sessionOwner, responseOwner, wantOwner, sessionErr, responseErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP session direct=%s: two Ready profiles; one %s attempt; exact body; HTTP 200; nested owner unchanged\n", p.name, shape, wantOwner)
	return nil
}

func verifyHTTPShadowedSession04383(shape string, result httpSoftConflictResult04383) error {
	wantOwner := "account-a"
	if httpShadowedSessionExpectSecond04383(shape) {
		wantOwner = "account-b"
	}
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpShadowedSessionReply04383 ||
		len(result.attempts) != 1 || !result.attempts[0].ready || result.attempts[0].owner != wantOwner ||
		result.attempts[0].turn != "" || result.attempts[0].metadata != httpShadowedSessionHeaderMetadata04383(shape) ||
		result.attempts[0].body != httpShadowedSessionBody04383(shape) {
		return fmt.Errorf("HTTP direct shadow metadata changed owner/wire: shape=%s invalid=%t status=%d attempts=%+v", shape, result.invalid, result.reply.status, result.attempts)
	}
	return nil
}
