package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const http429PolicyRequest04383 = `{"model":"gpt-6","input":[{"role":"user","content":"429 code policy probe"}]}`
const http429PolicyOK04383 = `{"id":"resp-429-policy","object":"response","output":[]}`

var http429PolicyShapes04383 = []string{
	"canonical-quota", "padded-code-rate", "overflow-unrelated-quota",
	"escaped-code-key-rate", "duplicate-rate-quota", "overdeep-rate",
	"oversized-pass", "invalid-utf8-pass",
}

func http429PolicyBody04383(shape string) string {
	switch shape {
	case "padded-code-rate":
		return `{"error":{"code":"insufficient_quota "}}`
	case "overflow-unrelated-quota":
		return `{"error":{"code":"insufficient_quota"},"unused":1e309}`
	case "escaped-code-key-rate":
		return `{"error":{"\u0063ode":"insufficient_quota"}}`
	case "duplicate-rate-quota":
		return `{"error":{"code":"rate_limit_exceeded","code":"insufficient_quota"}}`
	case "overdeep-rate":
		return strings.Repeat(`{"item":`, 34) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 34)
	case "oversized-pass":
		return `{"error":{"code":"insufficient_quota"},"padding":"` + strings.Repeat("x", 65536) + `"}`
	case "invalid-utf8-pass":
		return `{"error":{"code":"insufficient_quota"},"unused":"` + string([]byte{0xff}) + `"}`
	default:
		return `{"error":{"code":"insufficient_quota"}}`
	}
}

func runHTTP429PolicyProduct04383(root string, p product) error {
	for _, shape := range http429PolicyShapes04383 {
		if err := runHTTP429PolicyCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("HTTP 429 policy %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTP429PolicyCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-429-policy-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-429-policy-" + shape)
	defer backend.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 12 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		reply, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || reply.status != http.StatusOK {
			return fmt.Errorf("429 warmup %s status=%d err=%v", previous, reply.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("429 policy lacks authenticated Ready receipts on two profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", http429PolicyRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	attempts := append([]transportAttempt04383(nil), backend.attempts...)
	invalid := backend.invalid
	backend.mu.Unlock()
	if err := verifyHTTP429Policy04383(shape, reply, attempts, invalid); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		owner, err := readHTTPResponseOwner04383(p, "resp-429-policy")
		if err == nil && owner == "second" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("429 policy new response owner=%q want second err=%v", owner, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP429 %s: %d authenticated Ready attempts; final HTTP %d; exact original bytes\n", p.name, shape, len(attempts), reply.status)
	return nil
}

func verifyHTTP429Policy04383(shape string, reply exchange, attempts []transportAttempt04383, invalid bool) error {
	if invalid || len(attempts) != 2 || reply.status != http.StatusOK || reply.body != http429PolicyOK04383 {
		return fmt.Errorf("429 policy unexpected response/attempt count: shape=%s invalid=%t status=%d attempts=%+v body=%q", shape, invalid, reply.status, attempts, reply.body)
	}
	for index, attempt := range attempts {
		wantOwner := "account-a"
		if index == 1 {
			wantOwner = "account-b"
		}
		if attempt.owner != wantOwner || !attempt.ready || attempt.body != http429PolicyRequest04383 ||
			attempt.turn != "" || attempt.metadata != "" || attempt.trace != "" {
			return fmt.Errorf("429 policy changed owner/Ready/opaque request at %d: %+v", index, attempt)
		}
	}
	return nil
}
