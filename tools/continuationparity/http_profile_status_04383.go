package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const httpProfileStatusRequest04383 = `{"model":"gpt-6","input":[{"role":"user","content":"profile policy independent"}]}`
const httpProfileStatusSuccess04383 = `{"id":"resp-http-profile-status","object":"response","output":[]}`

func httpProfileStatusShapes04383() []string {
	return []string{
		"profile-ascii-403", "profile-uppercase-403", "profile-kelvin-403",
		"quota-text-ascii-403", "quota-text-kelvin-403",
		"quota-code-ascii-402", "quota-code-kelvin-402", "quota-text-uppercase-402", "unrelated-403",
		"quota-escaped-key-402", "quota-escaped-value-402", "quota-raw-depth-403", "quota-nested-control-403",
		"quota-scalar-error-402", "quota-escaped-scalar-error-402",
	}
}

func httpProfileStatusCode04383(shape string) int {
	switch shape {
	case "quota-code-ascii-402", "quota-code-kelvin-402", "quota-text-uppercase-402", "quota-escaped-key-402", "quota-escaped-value-402", "quota-scalar-error-402", "quota-escaped-scalar-error-402":
		return http.StatusPaymentRequired
	default:
		return http.StatusForbidden
	}
}

func httpProfileStatusBody04383(shape string) string {
	switch shape {
	case "profile-ascii-403":
		return `{"error":{"code":"deactivated_workspace"}}`
	case "profile-uppercase-403":
		return `{"error":{"code":"DEACTIVATED_WORKSPACE"}}`
	case "profile-kelvin-403":
		return `{"error":{"code":"deactivated_worKspace"}}`
	case "quota-text-ascii-403":
		return `{"error":{"message":"Your workspace is out of credits"}}`
	case "quota-text-kelvin-403":
		return `{"error":{"message":"Your worKspace is out of credits"}}`
	case "quota-code-ascii-402":
		return `{"error":{"code":"workspace_member_credits_depleted"}}`
	case "quota-code-kelvin-402":
		return `{"error":{"code":"worKspace_member_credits_depleted"}}`
	case "quota-text-uppercase-402":
		return `{"error":{"message":"YOUR WORKSPACE IS OUT OF CREDITS"}}`
	case "quota-escaped-key-402":
		return `{"error":{"\u0063ode":"insufficient_quota"}}`
	case "quota-escaped-value-402":
		return `{"error":{"code":"insufficient_\u0071uota"}}`
	case "quota-scalar-error-402":
		return `{"error":"insufficient_quota"}`
	case "quota-escaped-scalar-error-402":
		return `{"error":"insufficient_\u0071uota"}`
	case "quota-raw-depth-403":
		return strings.Repeat(`{"wrapper":`, 34) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 34)
	case "quota-nested-control-403":
		return strings.Repeat(`{"wrapper":`, 4) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 4)
	default:
		return `{"error":{"message":"unrelated profile denial"}}`
	}
}

func httpProfileStatusShouldRotate04383(shape string) bool {
	switch shape {
	case "profile-ascii-403", "profile-uppercase-403", "quota-text-ascii-403", "quota-code-ascii-402", "quota-text-uppercase-402", "quota-nested-control-403", "quota-scalar-error-402":
		return true
	default:
		return false
	}
}

func runHTTPProfileStatusProduct04383(root string, p product) error {
	for _, shape := range httpProfileStatusShapes04383() {
		if err := runHTTPProfileStatusCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("HTTP profile status %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPProfileStatusCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-profile-status-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-profile-status-" + shape)
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
			return fmt.Errorf("HTTP profile status warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(deadline) {
			return errors.New("profile status requires two authenticated Ready profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpProfileStatusRequest04383)
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), backend.attempts...), invalid: backend.invalid}
	backend.mu.Unlock()
	if err := verifyHTTPProfileStatus04383(shape, result); err != nil {
		return err
	}
	wantOwner := ""
	if httpProfileStatusShouldRotate04383(shape) {
		wantOwner = "second"
	}
	until := time.Now().Add(3 * time.Second)
	for {
		owner, err := readHTTPResponseOwner04383(p, "resp-http-profile-status")
		originalMain, oldErr := readHTTPResponseOwner04383(p, "resp-main")
		second, secondErr := readHTTPResponseOwner04383(p, "resp-ready-b")
		if err == nil && oldErr == nil && secondErr == nil && owner == wantOwner && originalMain == "main" && second == "second" {
			break
		}
		if time.Now().After(until) {
			return fmt.Errorf("HTTP profile status durable mismatch: new=%q want=%q main=%q second=%q errs=%v/%v/%v", owner, wantOwner, originalMain, second, err, oldErr, secondErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP %d profile %s: two Ready profiles; %d exact upstream attempts; terminal %d; durable new owner %q\n", p.name, httpProfileStatusCode04383(shape), shape, len(result.attempts), reply.status, wantOwner)
	return nil
}

func verifyHTTPProfileStatus04383(shape string, result httpSoftConflictResult04383) error {
	wantCount := 1
	wantCode, wantBody := httpProfileStatusCode04383(shape), httpProfileStatusBody04383(shape)
	if httpProfileStatusShouldRotate04383(shape) {
		wantCount = 2
		wantCode, wantBody = http.StatusOK, httpProfileStatusSuccess04383
	}
	if result.invalid || result.reply.status != wantCode || result.reply.body != wantBody || len(result.attempts) != wantCount {
		return fmt.Errorf("HTTP profile status error %s changed decision: invalid=%t status=%d body=%q attempts=%+v", shape, result.invalid, result.reply.status, result.reply.body, result.attempts)
	}
	for i, attempt := range result.attempts {
		owner := "account-a"
		if i == 1 {
			owner = "account-b"
		}
		if attempt.owner != owner || !attempt.ready || attempt.body != httpProfileStatusRequest04383 || attempt.turn != "" || attempt.trace != "" || attempt.metadata != "" {
			return fmt.Errorf("HTTP profile status %s changed Ready/owner/opaque input: attempt=%d %+v", shape, i, attempt)
		}
	}
	return nil
}
