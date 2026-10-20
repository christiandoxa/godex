package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const httpSSEInvalidErrorRequest04383 = `{"model":"gpt-6","stream":true,"previous_response_id":"resp-main","input":[{"role":"user","content":"independent invalid SSE"}]}`

func httpSSEInvalidErrorShapes04383() []string {
	return []string{"high-surrogate", "low-surrogate", "numeric-overflow", "too-deep", "valid-output"}
}

func httpSSEInvalidErrorBody04383(shape string) string {
	if shape == "valid-output" {
		return "data: " + `{"type":"response.output_text.delta","delta":"ok"}` + "\n\n"
	}
	note := `"\ud800"`
	switch shape {
	case "low-surrogate":
		note = `"\udc00"`
	case "numeric-overflow":
		note = `1e309`
	case "too-deep":
		note = strings.Repeat(`{"item":`, 128) + `0` + strings.Repeat(`}`, 128)
	}
	return "data: " + `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}},"unused":` + note + `}` + "\n\n"
}

func runHTTPSSEInvalidErrorProduct04383(root string, p product) error {
	for _, shape := range httpSSEInvalidErrorShapes04383() {
		if err := runHTTPSSEInvalidErrorCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("SSE invalid error %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPSSEInvalidErrorCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-sse-invalid-error-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-sse-invalid-error-" + shape)
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
			return fmt.Errorf("SSE invalid error warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("SSE invalid error lacks two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpSSEInvalidErrorRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, invalid: backend.invalid, attempts: append([]transportAttempt04383(nil), backend.attempts...)}
	backend.mu.Unlock()
	if err := verifyHTTPSSEInvalidError04383(shape, result); err != nil {
		return err
	}
	owner, err := readHTTPResponseOwner04383(p, "resp-main")
	if err != nil || owner != "main" {
		return fmt.Errorf("SSE invalid error changed existing verified previous response owner=%q err=%v", owner, err)
	}
	fmt.Printf("%s SSE invalid precommit %s: two applied Ready profiles; one main inference; HTTP 200 exact stream; previous owner retained\n", p.name, shape)
	return nil
}

func verifyHTTPSSEInvalidError04383(shape string, result httpSoftConflictResult04383) error {
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpSSEInvalidErrorBody04383(shape) ||
		len(result.attempts) != 1 || !result.attempts[0].ready || result.attempts[0].owner != "account-a" ||
		result.attempts[0].body != httpSSEInvalidErrorRequest04383 || result.attempts[0].turn != "" ||
		result.attempts[0].metadata != "" || result.attempts[0].trace != "" {
		return fmt.Errorf("SSE malformed event changed precommit owner, inference count or stream: %s %+v", shape, result)
	}
	return nil
}
