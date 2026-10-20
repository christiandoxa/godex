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
	owner, turn, body, metadata, trace string
	ready                              bool
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
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if strings.HasPrefix(b.scenario, "http-profile-status-") && string(body) == httpProfileStatusRequest04383 {
			shape := strings.TrimPrefix(b.scenario, "http-profile-status-")
			b.mu.Lock()
			prior := len(b.attempts)
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
				metadata: r.Header.Get("x-codex-turn-metadata"),
				ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			if prior == 0 && owner == "account-a" {
				writeJSON(w, httpProfileStatusCode04383(shape), httpProfileStatusBody04383(shape))
			} else {
				writeJSON(w, http.StatusOK, httpProfileStatusSuccess04383)
			}
			return
		}
		if strings.HasPrefix(b.scenario, "http-429-policy-") && string(body) == http429PolicyRequest04383 {
			shape := strings.TrimPrefix(b.scenario, "http-429-policy-")
			b.mu.Lock()
			prior := len(b.attempts)
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
				metadata: r.Header.Get("x-codex-turn-metadata"),
				ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			if prior == 0 && owner == "account-a" {
				if hint := http429PolicyHint04383(shape); hint != "" {
					w.Header().Set("X-Codex-Rate-Limit-Reached-Type", hint)
				}
				writeJSON(w, http.StatusTooManyRequests, http429PolicyBody04383(shape))
			} else {
				writeJSON(w, http.StatusOK, http429PolicyOK04383)
			}
			return
		}
		if strings.HasPrefix(b.scenario, "http-prompt-cache-") {
			shape := strings.TrimPrefix(b.scenario, "http-prompt-cache-")
			for _, stage := range []string{"seed-main", "seed-second", "restore-main", "fresh"} {
				if string(body) != httpPromptCacheRequest04383(stage) {
					continue
				}
				b.mu.Lock()
				b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
					turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
					metadata: r.Header.Get("x-codex-turn-metadata"),
					ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
				b.mu.Unlock()
				writeJSON(w, http.StatusOK, httpPromptCacheReply04383(shape, stage))
				return
			}
		}
		if strings.HasPrefix(b.scenario, "http-sse-turn-lineage-") && string(body) == httpSSETurnLineageRequest04383 {
			shape := strings.TrimPrefix(b.scenario, "http-sse-turn-lineage-")
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
				metadata: r.Header.Get("x-codex-turn-metadata"),
				ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			// The first output delta crosses the tagged SSE precommit
			// lookahead boundary. Only the second, later terminal event may
			// update subsequent response-turn lineage.
			wire := httpSSETurnLineageWireFor04383(shape)
			boundary := strings.Index(wire, "\n\n") + 2
			_, _ = io.WriteString(w, wire[:boundary])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(w, wire[boundary:])
			return
		}
		if strings.HasPrefix(b.scenario, "http-sse-invalid-error-") && string(body) == httpSSEInvalidErrorRequest04383 {
			shape := strings.TrimPrefix(b.scenario, "http-sse-invalid-error-")
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
				metadata: r.Header.Get("x-codex-turn-metadata"),
				ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if owner == "account-b" && httpSSEInvalidErrorRotates04383(shape) {
				_, _ = io.WriteString(w, httpSSERecovered04383)
			} else {
				_, _ = io.WriteString(w, httpSSEInvalidErrorBody04383(shape))
			}
			return
		}
		if strings.HasPrefix(b.scenario, "http-previous-text-") && string(body) == httpPreviousTextRequest04383 {
			shape := strings.TrimPrefix(b.scenario, "http-previous-text-")
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
				metadata: r.Header.Get("x-codex-turn-metadata"),
				ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			if owner == "account-b" {
				writeJSON(w, http.StatusOK, httpPreviousTextReply04383)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, httpPreviousTextBody04383(shape))
			return
		}
		if strings.HasPrefix(b.scenario, "http-recovery-session-") {
			shape := strings.TrimPrefix(b.scenario, "http-recovery-session-")
			// Capture every relevant recovery attempt, including a replay
			// that changes JSON serialization. The oracle verifies content
			// independently rather than letting an unknown replay silently
			// fall through as a generic warmup success.
			if string(body) == httpRecoverySessionRequest04383(shape) ||
				strings.Contains(string(body), `"content":"continue"`) {
				b.mu.Lock()
				b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
					turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
					metadata: r.Header.Get("x-codex-turn-metadata"),
					ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
				b.mu.Unlock()
				if strings.Contains(string(body), `"previous_response_id"`) {
					writeJSON(w, http.StatusBadRequest, httpRecoverySessionErrorForShape04383(shape))
				} else {
					writeJSON(w, http.StatusOK, httpRecoverySessionResponse04383)
				}
				return
			}
		}
		if strings.HasPrefix(b.scenario, "http-response-session-") {
			shape := strings.TrimPrefix(b.scenario, "http-response-session-")
			if string(body) == httpResponseSessionRequest04383(shape) {
				b.mu.Lock()
				b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
					turn: r.Header.Get("x-codex-turn-state"), trace: r.Header.Get("thread-id"),
					metadata: r.Header.Get("x-codex-turn-metadata"),
					ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
				b.mu.Unlock()
				if header := httpResponseSessionHeader04383(shape); header != "" {
					w.Header().Set("session-id", header)
				}
				writeJSON(w, http.StatusOK, httpResponseSessionReply04383(shape))
				return
			}
		}
		if strings.HasPrefix(b.scenario, "http-unsupported-thread-") {
			headerName := strings.TrimPrefix(b.scenario, "http-unsupported-thread-")
			for _, step := range httpUnsupportedThreadSteps04383 {
				if string(body) != step.body {
					continue
				}
				b.mu.Lock()
				b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
					trace: r.Header.Get(headerName), turn: r.Header.Get("x-codex-turn-state"),
					metadata: r.Header.Get("x-codex-turn-metadata"),
					ready:    b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
				b.mu.Unlock()
				writeJSON(w, http.StatusOK, httpUnsupportedThreadReply04383(step.responseID))
				return
			}
		}
		if (strings.HasPrefix(b.scenario, "http-previous-session-") ||
			strings.HasPrefix(b.scenario, "http-turn-session-")) &&
			strings.Contains(string(body), `"session_id":"http-hard-session"`) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body), turn: r.Header.Get("x-codex-turn-state"),
				ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-hard-previous","object":"response","output":[]}`)
			return
		}
		if strings.HasPrefix(b.scenario, "http-nested-prev-") && strings.Contains(string(body), `"response":{"previous_response_id":"resp-nested"}`) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body), turn: r.Header.Get("x-codex-turn-state"), ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-nested-previous","object":"response","output":[]}`)
			return
		}
		if b.scenario == "http-session-conflict" && strings.Contains(string(body), `"session_id":"http-conflict"`) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner, body: string(body),
				ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-soft-conflict","object":"response","output":[]}`)
			return
		}
		if strings.HasPrefix(b.scenario, "turn-http-shadow-") &&
			strings.Contains(string(body), `"client_metadata":{"x-codex-turn-state":"shadow-turn"}`) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner,
				body: string(body), turn: r.Header.Get("x-codex-turn-state"),
				ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-turn-shadow","object":"response","output":[]}`)
			return
		}
		if strings.HasPrefix(b.scenario, "session-http-shadow-") &&
			string(body) == httpShadowedSessionBody04383(strings.TrimPrefix(b.scenario, "session-http-shadow-")) {
			b.mu.Lock()
			b.attempts = append(b.attempts, transportAttempt04383{owner: owner,
				body: string(body), metadata: r.Header.Get("x-codex-turn-metadata"), turn: r.Header.Get("x-codex-turn-state"),
				ready: b.receipts["account-a"] > 0 && b.receipts["account-b"] > 0})
			b.mu.Unlock()
			writeJSON(w, http.StatusOK, `{"id":"resp-http-shadow","object":"response","output":[]}`)
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
	if strings.HasPrefix(b.scenario, "tool-suffix-") {
		_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"error","status":400,"error":{"code":"previous_response_not_found","message":"synthetic missing response"}}`), false)
		return
	}
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
	if b.scenario == "nested-previous-only" || b.scenario == "fresh-websocket-session" || b.scenario == "compact-lineage" || b.scenario == "compact-turn-conflict" || b.scenario == "compact-previous-conflict" || b.scenario == "compact-unbound" || strings.HasPrefix(b.scenario, "compact-produced") || strings.HasPrefix(b.scenario, "compact-shadow-") || strings.HasPrefix(b.scenario, "session-") || strings.HasPrefix(b.scenario, "compact-session-") {
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
