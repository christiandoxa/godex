package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func runWebSocketTransportProduct04383(root string, p product) error {
	for _, scenario := range []string{"full-history", "unbound-history", "quota-history", "unbound-quota-history", "turn-missing", "delta-only", "unbound-delta", "unbound-quota-delta", "previous-response", "postcommit", "cancellation", "deadline"} {
		if err := runTransportCase04383(filepath.Join(root, scenario), p, scenario); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	return nil
}

func runTransportCase04383(root string, p product, scenario string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, scenario); err != nil {
		return err
	}
	if strings.HasPrefix(scenario, "unbound-") {
		owners, err := readTransportOwners04383(p)
		if err != nil || owners["turn"] != "" {
			return fmt.Errorf("unbound fixture acquired a turn owner: owners=%v err=%v", owners, err)
		}
	}
	p.env = append(p.env, "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000", "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000", "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383(scenario)
	defer b.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 5 * time.Second}
	warmups := []string{"resp-main", "resp-ready-b"}
	if strings.HasPrefix(scenario, "unbound-") {
		warmups[0], warmups[1] = warmups[1], warmups[0]
	}
	for _, previous := range warmups {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != 200 || !strings.Contains(warmup.body, "resp-warmup") {
			return fmt.Errorf("owner warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	readyDeadline := time.Now().Add(5 * time.Second)
	for {
		if transportProfilesReady04383(p, b) {
			break
		}
		if time.Now().After(readyDeadline) {
			b.mu.Lock()
			err := fmt.Errorf("no authenticated Ready receipts for both profiles: receipts=%v warmups=%d invalid=%t fixture=%s", b.receipts, b.warmups, b.invalid, p.root)
			b.mu.Unlock()
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	connection, reader, err := wsConnectHeaders04383(endpoint+"/responses", "x-codex-turn-state: turn-a\r\nsession_id: sess-transport\r\n")
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(12 * time.Second))
	body := transportHistory04383
	if scenario == "delta-only" || strings.HasSuffix(scenario, "-delta") {
		body = `{"type":"response.create","model":"gpt-6","session_id":"sess-transport","client_metadata":{"x-codex-turn-state":"turn-a"},"input":[{"type":"function_call_output","call_id":"call-large","output":"done"}]}`
	}
	if scenario == "previous-response" {
		body = strings.Replace(body, `"type":"response.create"`, `"type":"response.create","previous_response_id":"resp-main"`, 1)
	}
	if err := websocketframe.WriteFrame(connection, 1, []byte(body), true); err != nil {
		return err
	}
	if scenario == "deadline" || scenario == "cancellation" || scenario == "postcommit" {
		return runTransportLifecycle04383(p, b, connection, reader)
	}
	var frames []string
	for len(frames) < 2 {
		opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
		if err != nil {
			return fmt.Errorf("transport recovery client: %w; frames=%q", err, frames)
		}
		if opcode != 1 {
			return fmt.Errorf("unexpected client opcode %d", opcode)
		}
		frames = append(frames, string(payload))
		if scenario == "delta-only" || strings.HasSuffix(scenario, "-delta") || scenario == "previous-response" {
			b.mu.Lock()
			err := verifyTransportNegative04383(scenario, body, b.handshakes, b.attempts, string(payload), b.invalid)
			b.mu.Unlock()
			if err != nil {
				return err
			}
			if err := verifyTransportNegativeOwners04383(p, scenario); err != nil {
				return err
			}
			fmt.Printf("%s %s: authenticated Ready alternate; one account-a attempt; terminal=%s\n", p.name, scenario, payload)
			return nil
		}
		if strings.Contains(string(payload), `"type":"error"`) {
			return fmt.Errorf("transport recovery error: %s", payload)
		}
	}
	b.mu.Lock()
	err = verifyTransportRecovery04383(scenario, b.handshakes, b.attempts, frames, b.invalid)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	recoveryEvent := "transport_failure_turn_state_full_context_replay"
	if scenario == "quota-history" || scenario == "unbound-quota-history" {
		recoveryEvent = "quota_blocked_turn_state_full_context_replay"
	}
	if scenario == "turn-missing" {
		recoveryEvent = "previous_response_not_found"
	}
	if err := verifyTransportDurable04383(p, scenario, recoveryEvent); err != nil {
		return err
	}
	fmt.Printf("%s %s: authenticated Ready owners=2; handshakes=2; requests=account-a,account-b; exact turn policy/history/client frames preserved\n", p.name, scenario)
	return nil
}

func verifyTransportNegative04383(scenario, body string, handshakes []string, attempts []transportAttempt04383, payload string, invalid bool) error {
	if invalid || !reflect.DeepEqual(handshakes, []string{"account-a"}) || len(attempts) != 1 ||
		!attempts[0].ready || attempts[0].owner != "account-a" || attempts[0].turn != "turn-a" || !sameTransportJSON04383(attempts[0].body, body) {
		return fmt.Errorf("%s changed owner/opaque input or replayed: handshakes=%v attempts=%+v invalid=%t", scenario, handshakes, attempts, invalid)
	}
	want := `{"error":{"code":"service_unavailable","message":"Runtime proxy could not secure a healthy upstream profile before the pre-commit retry budget was exhausted. Retry the request."},"status":503,"type":"error"}`
	if scenario == "unbound-quota-delta" {
		want = `{"type":"error","status":429,"error":{"code":"insufficient_quota","message":"synthetic quota"}}`
	}
	if scenario == "previous-response" {
		want = `{"error":{"code":"previous_response_not_found","message":"Previous response was not found. Retrying the full request."},"status":400,"type":"error"}`
	}
	if payload != want {
		return fmt.Errorf("%s terminal frame differs: got=%s want=%s", scenario, payload, want)
	}
	return nil
}

func sameTransportJSON04383(left, right string) bool {
	if !json.Valid([]byte(left)) || !json.Valid([]byte(right)) {
		return false
	}
	var a, b any
	first := json.NewDecoder(strings.NewReader(left))
	first.UseNumber()
	second := json.NewDecoder(strings.NewReader(right))
	second.UseNumber()
	return first.Decode(&a) == nil && second.Decode(&b) == nil && reflect.DeepEqual(a, b)
}

func verifyTransportRecovery04383(scenario string, handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	if invalid || !reflect.DeepEqual(handshakes, []string{"account-a", "account-b"}) || len(attempts) != 2 {
		return fmt.Errorf("transport auth/handshakes/attempt count changed: invalid=%t handshakes=%v attempts=%d", invalid, handshakes, len(attempts))
	}
	if !reflect.DeepEqual(frames, []string{transportDelta04383, transportCompleted04383}) {
		return fmt.Errorf("transport leaked prelude or changed client frames: %q", frames)
	}
	for index, attempt := range attempts {
		if !attempt.ready || attempt.owner != handshakes[index] {
			return fmt.Errorf("attempt %d lacks authenticated Ready evidence or changed owner", index)
		}
		wantTurn := "turn-a"
		if index == 1 && scenario != "turn-missing" {
			wantTurn = ""
		}
		if attempt.turn != wantTurn {
			return fmt.Errorf("attempt %d changed turn-state header: %q", index, attempt.turn)
		}
		var got, want map[string]any
		actual := json.NewDecoder(strings.NewReader(attempt.body))
		actual.UseNumber()
		expected := json.NewDecoder(strings.NewReader(transportHistory04383))
		expected.UseNumber()
		if actual.Decode(&got) != nil || expected.Decode(&want) != nil {
			return errors.New("invalid transport request JSON")
		}
		if index == 1 && scenario != "turn-missing" {
			delete(want, "x-codex-turn-state")
			delete(want["client_metadata"].(map[string]any), "x-codex-turn-state")
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("attempt %d changed opaque full history or retained stale metadata: %s", index, attempt.body)
		}
	}
	return nil
}
