package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// These are unbound, independent first-turn SSE requests. A hard
// previous_response_id would forbid cross-profile replay for a different
// reason and mask the STREAM error-code classifier under test.
const httpSSEInvalidErrorRequest04383 = `{"model":"gpt-6","stream":true,"input":[{"role":"user","content":"independent invalid SSE"}]}`

func httpSSEInvalidErrorShapes04383() []string {
	return []string{"high-surrogate", "low-surrogate", "numeric-overflow", "too-deep", "valid-output",
		"opaque-scalar-quota", "opaque-error-code-quota", "opaque-error-code-rate",
		"opaque-profile-note", "opaque-profile-message", "kelvin-quota-code",
		"canonical-quota", "canonical-rate", "created-quota", "inprogress-rate", "keepalive-quota",
		"quota-and-invalid-id", "rate-and-invalid-id", "reason-quota-and-invalid-id"}
}

func httpSSEInvalidErrorBody04383(shape string) string {
	switch shape {
	case "opaque-scalar-quota":
		return "data: " + `{"type":"response.failed","response":{"error":"insufficient_quota"}}` + "\n\n"
	case "opaque-error-code-quota":
		return "data: " + `{"type":"response.failed","response":{"error":{"error_code":"insufficient_quota"}}}` + "\n\n"
	case "opaque-error-code-rate":
		return "data: " + `{"type":"response.failed","response":{"error":{"error_code":"rate_limit_exceeded"}}}` + "\n\n"
	case "opaque-profile-note":
		return "data: " + `{"type":"response.failed","response":{"error":{"note":"deactivated_workspace"}}}` + "\n\n"
	case "opaque-profile-message":
		return "data: " + `{"type":"response.failed","response":{"error":{"message":"deactivated_workspace"}}}` + "\n\n"
	case "kelvin-quota-code":
		return "data: " + `{"type":"response.failed","response":{"error":{"code":"worKspace_member_credits_depleted"}}}` + "\n\n"
	case "canonical-quota":
		return "data: " + `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}` + "\n\n"
	case "canonical-rate":
		return "data: " + `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded"}}}` + "\n\n"
	case "created-quota":
		return "data: " + `{"type":"response.created","response":{"error":{"code":"insufficient_quota"}}}` + "\n\n"
	case "inprogress-rate":
		return "data: " + `{"type":"response.in_progress","response":{"error":{"code":"rate_limit_exceeded"}}}` + "\n\n"
	case "keepalive-quota":
		return "data: " + `{"type":"keepalive","error":{"code":"insufficient_quota"}}` + "\n\n"
	case "quota-and-invalid-id":
		return "data: " + `{"type":"response.failed","response":{"error":{"code":"insufficient_quota","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}` + "\n\n"
	case "rate-and-invalid-id":
		return "data: " + `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}` + "\n\n"
	case "reason-quota-and-invalid-id":
		return "data: " + `{"type":"response.failed","response":{"error":{"reason":"insufficient_quota","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}}` + "\n\n"
	}
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

func httpSSEInvalidErrorRotates04383(shape string) bool {
	return shape == "canonical-quota" || shape == "canonical-rate" ||
		shape == "created-quota" || shape == "inprogress-rate" || shape == "keepalive-quota" ||
		shape == "quota-and-invalid-id" || shape == "rate-and-invalid-id" || shape == "reason-quota-and-invalid-id"
}

const httpSSERecovered04383 = "data: " + `{"type":"response.output_text.delta","delta":"recovered"}` + "\n\n"

func httpSSEInvalidErrorClientBody04383(shape string) string {
	if httpSSEInvalidErrorRotates04383(shape) {
		return httpSSERecovered04383
	}
	return httpSSEInvalidErrorBody04383(shape)
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
	fmt.Printf("%s SSE precommit %s: two applied Ready profiles; %d account-specific model attempts; HTTP 200 exact stream; previous owner retained\n", p.name, shape, len(result.attempts))
	return nil
}

func verifyHTTPSSEInvalidError04383(shape string, result httpSoftConflictResult04383) error {
	wantAttempts := 1
	if httpSSEInvalidErrorRotates04383(shape) {
		wantAttempts = 2
	}
	if result.invalid || result.reply.status != http.StatusOK || result.reply.body != httpSSEInvalidErrorClientBody04383(shape) ||
		len(result.attempts) != wantAttempts {
		return fmt.Errorf("SSE precommit %s changed account count or client stream: %+v", shape, result)
	}
	for index, attempt := range result.attempts {
		want := "account-a"
		if index == 1 {
			want = "account-b"
		}
		if !attempt.ready || attempt.owner != want || attempt.body != httpSSEInvalidErrorRequest04383 ||
			attempt.turn != "" || attempt.metadata != "" || attempt.trace != "" {
			return fmt.Errorf("SSE precommit %s wrong owner/Ready/opaque wire on attempt %d: %+v", shape, index, attempt)
		}
	}
	return nil
}
