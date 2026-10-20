package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const httpRecoverySessionResponse04383 = `{"id":"resp-recovery-success","object":"response","output":[]}`
const httpRecoverySessionError04383 = "{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}"
const httpRecoverySessionHistory04383 = `"input":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"continue"}]`

func httpRecoverySessionRequest04383(shape string) string {
	field := `"session_id":null,"client_metadata":{"session_id":"verified-session"},`
	switch shape {
	case "blank":
		field = `"session_id":"   ","client_metadata":{"session_id":"verified-session"},`
	case "nested":
		field = `"client_metadata":{"session_id":"verified-session"},`
	case "direct":
		field = `"session_id":"verified-session",`
	case "metadata-null", "metadata-nested", "metadata-invalid", "metadata-valid":
		field = ""
	case "invalid-utf8", "invalid-surrogate", "invalid-number", "invalid-depth", "valid-unicode", "valid-finite":
		field = `"session_id":"verified-session",`
	case "header-override":
		field = `"session_id":null,`
	}
	note := ""
	switch shape {
	case "invalid-utf8":
		note = `,"unrelated":"bad-` + string([]byte{0xff}) + `"`
	case "invalid-surrogate":
		note = `,"unrelated":"\ud800"`
	case "invalid-number":
		note = `,"unrelated":1e309`
	case "invalid-depth":
		note = `,"unrelated":` + strings.Repeat(`{"child":`, 128) + `0` + strings.Repeat(`}`, 128)
	case "valid-unicode":
		note = `,"unrelated":"\ud83d\ude00"`
	case "valid-finite":
		note = `,"unrelated":1e308`
	}
	return `{"model":"gpt-6","previous_response_id":"resp-main",` + field + httpRecoverySessionHistory04383 + note + `}`
}

func httpRecoverySessionMetadata04383(shape string) string {
	switch shape {
	case "metadata-null":
		return `{"session_id":null,"client_metadata":{"session_id":"verified-session"}}`
	case "metadata-nested", "header-override":
		return `{"client_metadata":{"session_id":"verified-session"}}`
	case "metadata-invalid":
		return `{"session_id":"verified-session","note":"\ud800"}`
	case "metadata-valid":
		return `{"session_id":"verified-session","note":"\ud83d\ude00"}`
	}
	return ""
}

func httpRecoverySessionExpectRetry04383(shape string) bool {
	return shape == "nested" || shape == "direct" || shape == "metadata-nested" || shape == "header-override" ||
		shape == "valid-unicode" || shape == "valid-finite" || shape == "metadata-valid"
}

func httpRecoverySessionReplay04383(shape string) string {
	var request map[string]json.RawMessage
	if err := json.Unmarshal([]byte(httpRecoverySessionRequest04383(shape)), &request); err != nil {
		panic(err)
	}
	delete(request, "previous_response_id")
	out, err := json.Marshal(request)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func runHTTPRecoverySessionProduct04383(root string, p product) error {
	for _, shape := range []string{"null", "blank", "nested", "direct", "metadata-null", "metadata-nested", "header-override",
		"invalid-utf8", "invalid-surrogate", "invalid-number", "invalid-depth", "valid-unicode", "valid-finite", "metadata-invalid", "metadata-valid"} {
		if err := runHTTPRecoverySessionCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("HTTP full-history recovery %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPRecoverySessionCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-recovery-session-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("http-recovery-session-" + shape)
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
			return fmt.Errorf("HTTP recovery warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(until) {
			return errors.New("HTTP recovery fixture lacks two applied authenticated Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	body := httpRecoverySessionRequest04383(shape)
	var reply exchange
	if header := httpRecoverySessionMetadata04383(shape); header != "" {
		reply, err = postContextHeader(client, endpoint+"/responses", body, "x-codex-turn-metadata", header)
	} else {
		reply, err = post(client, endpoint+"/responses", body)
	}
	if err != nil {
		return err
	}
	backend.mu.Lock()
	result := httpSoftConflictResult04383{reply: reply, invalid: backend.invalid, attempts: append([]transportAttempt04383(nil), backend.attempts...)}
	backend.mu.Unlock()
	if err := verifyHTTPRecoverySession04383(shape, result); err != nil {
		return err
	}
	wantResponseOwner := ""
	if httpRecoverySessionExpectRetry04383(shape) {
		wantResponseOwner = "main"
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		owner, err := readHTTPResponseOwner04383(p, "resp-recovery-success")
		if err == nil && owner == wantResponseOwner {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTP recovery durable response owner=%q want=%q err=%v", owner, wantResponseOwner, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP recovery session %s: two Ready profiles; %d exact main attempts; terminal=%d; correct durable recovered response\n", p.name, shape, len(result.attempts), reply.status)
	return nil
}

func verifyHTTPRecoverySession04383(shape string, result httpSoftConflictResult04383) error {
	want := 1
	wantStatus, wantBody := http.StatusBadRequest, httpRecoverySessionError04383
	if httpRecoverySessionExpectRetry04383(shape) {
		want = 2
		wantStatus, wantBody = http.StatusOK, httpRecoverySessionResponse04383
	}
	if result.invalid || result.reply.status != wantStatus || result.reply.body != wantBody || len(result.attempts) != want {
		return fmt.Errorf("HTTP full-history recovery wrong replay/status: shape=%s invalid=%t status=%d response=%q attempts=%+v", shape, result.invalid, result.reply.status, result.reply.body, result.attempts)
	}
	for index, attempt := range result.attempts {
		reference := httpRecoverySessionRequest04383(shape)
		if index == 1 {
			reference = httpRecoverySessionReplay04383(shape)
		}
		matchesWire := attempt.body == reference
		if index == 1 {
			// Recovery reserializes valid JSON. Preserve semantic equality
			// while requiring byte-exact forwarding of the original request,
			// including malformed JSON which must never be rewritten.
			matchesWire = sameTransportJSON04383(attempt.body, reference)
			if shape == "valid-finite" && !matchesWire {
				// serde_json 1.0.151 prints the *same* finite floating
				// exponent with an explicit positive exponent sign (1e+308),
				// whereas Go RawMessage retains its original text (1e308).
				// Only this one known numeric token may be normalized; all
				// other numeric/string fields stay exact under UseNumber.
				matchesWire = sameTransportJSON04383(
					strings.ReplaceAll(attempt.body, `"unrelated":1e+308`, `"unrelated":1e308`), reference)
			}
		}
		if attempt.owner != "account-a" || !attempt.ready || attempt.turn != "" || attempt.metadata != httpRecoverySessionMetadata04383(shape) ||
			attempt.trace != "" || !matchesWire {
			return fmt.Errorf("HTTP recovery %s changed account/Ready/replay wire: attempt=%d %+v", shape, index, attempt)
		}
		if index == 0 && attempt.body != httpRecoverySessionRequest04383(shape) {
			return errors.New("HTTP recovery initial body was not byte-for-byte preserved")
		}
		if index == 1 && strings.Contains(attempt.body, `"previous_response_id"`) {
			return errors.New("HTTP recovery replay retained invalid previous-response reference")
		}
	}
	return nil
}
