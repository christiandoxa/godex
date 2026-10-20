package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const transportHistory04383 = `{"type":"response.create","model":"gpt-6","session_id":"sess-transport","x-codex-turn-state":"turn-a","client_metadata":{"x-codex-turn-state":"turn-a","sequence":18446744073709551615},"input":[{"type":"message","role":"user","content":"first","sequence":9007199254740993},{"type":"message","role":"assistant","content":"answer"},{"type":"function_call","call_id":"call-large","name":"lookup","arguments":"{\"id\":9007199254740993}"},{"type":"function_call_output","call_id":"call-large","output":"done"},{"type":"message","role":"user","content":"next"}]}`
const transportDelta04383 = `{"type":"response.output_text.delta","delta":"recovered"}`
const transportCompleted04383 = `{"type":"response.completed","response":{"id":"resp-transport-recovered"}}`

type transportAttempt04383 struct {
	owner, turn, body string
	ready             bool
}

type transportBackend04383 struct {
	server        *httptest.Server
	scenario      string
	mu            sync.Mutex
	receipts      map[string]int
	handshakes    []string
	attempts      []transportAttempt04383
	compactOwners []string
	invalid       bool
	warmups       int
	canceled      chan struct{}
	busy          chan struct{}
	occupied      bool
}

func newTransportBackend04383(scenario string) *transportBackend04383 {
	b := &transportBackend04383{scenario: scenario, receipts: make(map[string]int), canceled: make(chan struct{}, 1), busy: make(chan struct{}, 1)}
	b.server = httptest.NewServer(http.HandlerFunc(b.serve))
	return b
}

func (b *transportBackend04383) serve(w http.ResponseWriter, r *http.Request) {
	owner := r.Header.Get("ChatGPT-Account-Id")
	expected := map[string]string{"account-a": "Bearer token-a", "account-b": "Bearer token-b"}[owner]
	authenticated := expected != "" && r.Header.Get("Authorization") == expected
	b.mu.Lock()
	b.invalid = b.invalid || !authenticated
	b.mu.Unlock()
	if !authenticated {
		http.Error(w, "invalid synthetic auth", 401)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/wham/usage") {
		b.mu.Lock()
		b.receipts[owner]++
		b.mu.Unlock()
		writeJSON(w, 200, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":10,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":10,"reset_at":4102444800,"limit_window_seconds":604800}}}`)
		return
	}
	if strings.HasPrefix(b.scenario, "compact-produced") && strings.HasSuffix(r.URL.Path, "/responses/compact") && r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		b.mu.Lock()
		b.compactOwners = append(b.compactOwners, owner)
		b.invalid = b.invalid || owner != "account-b" || !strings.Contains(string(body), `"session_id":"compact-produced-session"`) || !strings.Contains(string(body), `"previous_response_id":"resp-ready-b"`)
		b.mu.Unlock()
		turn := "compact-turn"
		if b.scenario == "compact-produced-long-turn" {
			turn = strings.Repeat("L", 1025)
		}
		w.Header().Set("x-codex-turn-state", turn)
		writeJSON(w, http.StatusOK, `{"id":"resp-compact-produced","object":"response","output":[]}`)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/responses") && r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		if b.scenario == "http-session-conflict" && strings.Contains(string(body), `"session_id":"http-conflict"`) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-soft-conflict","object":"response","output":[]}`)
			return
		}
		if strings.Contains(string(body), `"mode":"capacity-blocker"`) {
			b.holdCapacity04383(w, r)
			return
		}
		b.mu.Lock()
		b.warmups++
		b.mu.Unlock()
		writeJSON(w, 200, fmt.Sprintf(`{"id":"resp-warmup-%s","object":"response","output":[]}`, owner))
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/responses") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "unexpected synthetic route", 404)
		return
	}
	connection, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	b.mu.Lock()
	b.handshakes = append(b.handshakes, owner)
	b.mu.Unlock()
	digest := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
	if buffered.Flush() != nil {
		return
	}
	frame, err := websocketframe.ReadHeader(buffered.Reader)
	if err != nil {
		return
	}
	payload, err := frame.ReadPayload(buffered.Reader, 1<<20)
	if err != nil {
		return
	}
	frame.Unmask(payload)
	b.mu.Lock()
	b.invalid = b.invalid || frame.Opcode != 1 || !frame.Masked()
	b.attempts = append(b.attempts, transportAttempt04383{owner: owner, turn: r.Header.Get("x-codex-turn-state"), body: string(payload), ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
	b.mu.Unlock()
	if b.scenario == "compact-bound-transport" && owner == "account-b" {
		// The verified Compact owner closes before any client-visible model
		// event. A reconstructable full history can recover on Ready main.
		_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"response.created","response":{"id":"discarded-compact-precommit"}}`), false)
		return
	}
	if b.scenario == "compact-bound-transport" && owner == "account-a" {
		// After a failed second-owned precommit socket, the Ready main
		// alternate completes the full-history request. This separates
		// safe replay from the independently tested double-EOF deadline.
		for _, event := range []string{transportDelta04383, transportCompleted04383} {
			if websocketframe.WriteFrame(connection, 1, []byte(event), false) != nil {
				return
			}
		}
		return
	}
	if b.scenario == "fresh-websocket-session" || b.scenario == "compact-lineage" || b.scenario == "compact-turn-conflict" || b.scenario == "compact-previous-conflict" || b.scenario == "compact-unbound" || strings.HasPrefix(b.scenario, "compact-produced") || strings.HasPrefix(b.scenario, "compact-shadow-") || strings.HasPrefix(b.scenario, "session-") || strings.HasPrefix(b.scenario, "compact-session-") {
		for _, event := range []string{transportDelta04383, transportCompleted04383} {
			if websocketframe.WriteFrame(connection, 1, []byte(event), false) != nil {
				return
			}
		}
		return
	}
	if owner == "account-a" {
		if b.scenario == "turn-missing" {
			_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"error","status":400,"error":{"code":"previous_response_not_found","message":"synthetic missing"}}`), false)
			return
		}
		if b.scenario == "deadline" {
			_ = connection.SetDeadline(time.Now().Add(40 * time.Second))
			serveHiddenPreludeDeadline04382(connection)
			return
		}
		if b.scenario == "cancellation" {
			_, err := connection.Read(make([]byte, 1))
			if err != nil {
				select {
				case b.canceled <- struct{}{}:
				default:
				}
			}
			return
		}
		// This metadata stays below the commit threshold; EOF is deterministic.
		_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"response.created","response":{"id":"discarded-transport"}}`), false)
		if frame := retryableFailureFrame04383(b.scenario); frame != "" {
			_ = websocketframe.WriteFrame(connection, 1, []byte(frame), false)
			return
		}
		if b.scenario == "quota-history" || strings.HasPrefix(b.scenario, "unbound-quota-") {
			_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"error","status":429,"error":{"code":"insufficient_quota","message":"synthetic quota"}}`), false)
		}
		if b.scenario == "postcommit" {
			_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"response.output_text.delta","delta":"visible"}`), false)
		}
		return
	}
	for _, event := range []string{transportDelta04383, transportCompleted04383} {
		if websocketframe.WriteFrame(connection, 1, []byte(event), false) != nil {
			return
		}
	}
}

func retryableFailureFrame04383(scenario string) string {
	switch scenario {
	case "previous-quota", "unknown-quota":
		return `{"type":"error","status":429,"error":{"code":"insufficient_quota","message":"synthetic quota"}}`
	case "previous-rate", "unknown-rate":
		return `{"type":"error","status":429,"error":{"code":"rate_limit_exceeded","message":"synthetic rate"}}`
	case "previous-overload", "unknown-overload":
		return `{"type":"error","status":503,"error":{"code":"server_is_overloaded","message":"synthetic overload"}}`
	}
	return ""
}
