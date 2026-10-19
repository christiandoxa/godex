package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const (
	deadlineMode04382         = "ws-hidden-deadline-04382"
	deadlineFollowupMode04382 = "ws-followup-deadline-04382"
	deadlineBudget04382       = 30 * time.Second
)

const deadlineMessage04382 = "request timed out"

// All status and wire fields must stay fixed across both binaries. The only
// tolerated variation is elapsed wall time within the small deterministic
// deadline window. Never normalize an extra dispatch or changed owner.
type websocketDeadlineResult04382 struct {
	code, message, completed string
	status, handshakes       int
	heartbeats               int
	elapsed                  time.Duration
	attempts                 []attempt
}

func verifyWebSocketDeadline04382(result websocketDeadlineResult04382) error {
	if result.status != 504 || result.code != "request_timeout" || result.message != deadlineMessage04382 {
		return fmt.Errorf("precommit deadline status/code/message=%d/%q/%q", result.status, result.code, result.message)
	}
	if result.heartbeats < 1 || result.heartbeats > 40 {
		return fmt.Errorf("precommit hidden metadata was not kept alive by 0.438.3 Ping frames: %d", result.heartbeats)
	}
	if result.elapsed < deadlineBudget04382-2500*time.Millisecond || result.elapsed > 31*time.Second {
		return fmt.Errorf("precommit watchdog shifted from original 30s: %s", result.elapsed)
	}
	if result.completed != "resp-after-watchdog" || result.handshakes != 2 || len(result.attempts) != 2 {
		return fmt.Errorf("precommit deadline lost socket/turn: complete=%q handshakes=%d attempts=%d", result.completed, result.handshakes, len(result.attempts))
	}
	expected := []string{deadlineMode04382, deadlineFollowupMode04382}
	for index, one := range result.attempts {
		body := fmt.Sprintf(`{"type":"response.create","mode":%q,"input":[]}`, expected[index])
		if one.account != "account-a" || one.mode != expected[index] || one.body != body {
			return fmt.Errorf("precommit deadline mutated upstream[%d]: account=%q mode=%q body=%q", index, one.account, one.mode, one.body)
		}
	}
	return nil
}

// Prodex 0.438.2 applies min(socket I/O timeout, ORIGINAL remaining precommit
// deadline) on *each* upstream send/read. A stream of hidden keepalives ends
// BEFORE the 30-second deadline; with no new frames the last blocked read
// must still terminate by the original deadline, not by another fresh eight
// seconds of progress timeout. The client can use its local WebSocket again.
func serveHiddenPreludeDeadline04382(conn net.Conn) {
	const prelude = `{"type":"keepalive"}`
	began := time.Now()
	for time.Since(began) < 29*time.Second {
		if websocketframe.WriteFrame(conn, 1, []byte(prelude), false) != nil {
			return
		}
		time.Sleep(245 * time.Millisecond)
	}
	// Keep the old upstream open *past* the request deadline; neither a
	// 502 transport failure nor early EOF may masquerade as an honest 504.
	_ = conn.SetReadDeadline(time.Now().Add(13 * time.Second))
	_, _ = conn.Read(make([]byte, 1))
}

func runWebSocketDeadlineProduct04382(root string, b *backend, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	env := make([]string, 0, len(p.env)+2)
	for _, value := range p.env {
		if strings.HasPrefix(value, "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=") ||
			strings.HasPrefix(value, "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=") {
			continue
		}
		env = append(env, value)
	}
	// Prodex derives max(stream_idle_ms, 30_000ms). Both published binaries
	// use their real runtime environment, no monkeypatch or fake clock.
	p.env = append(env,
		"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000",
		"GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000",
	)
	endpoint, process, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stopGateway(process)
	b.mu.Lock()
	baseline := len(b.attempts)
	startHandshakes := b.wsHandshakes
	b.mu.Unlock()
	conn, reader, err := wsConnect(endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	// A correct product finishes near 30 seconds, while a new eight-second
	// idle grant would exceed this safety bound after the final keepalive.
	_ = conn.SetDeadline(time.Now().Add(34 * time.Second))
	start := time.Now()
	if err := websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.create","mode":"`+deadlineMode04382+`","input":[]}`), true); err != nil {
		return err
	}
	code, status, message, heartbeatCount, err := readDeadlineError04382(conn, reader)
	elapsed := time.Since(start)
	if err != nil {
		return fmt.Errorf("%s: 0.438.2 hidden precommit deadline: %w", p.name, err)
	}
	if status != 504 || code != "request_timeout" || message != deadlineMessage04382 ||
		elapsed < deadlineBudget04382-2500*time.Millisecond || elapsed > 31*time.Second {
		return fmt.Errorf("%s: first precommit timeout %d/%q/%q elapsed=%s", p.name, status, code, message, elapsed)
	}
	if err := websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.create","mode":"`+deadlineFollowupMode04382+`","input":[]}`), true); err != nil {
		return fmt.Errorf("%s: local WebSocket not reusable after 504: %w", p.name, err)
	}
	completed, err := readDeadlineCompleted04382(conn, reader)
	if err != nil {
		return fmt.Errorf("%s: followup turn after 504: %w", p.name, err)
	}
	b.mu.Lock()
	result := websocketDeadlineResult04382{
		code: code, status: status, message: message, completed: completed, elapsed: elapsed, heartbeats: heartbeatCount,
		handshakes: b.wsHandshakes - startHandshakes,
		attempts:   append([]attempt(nil), b.attempts[baseline:]...),
	}
	b.mu.Unlock()
	if err := verifyWebSocketDeadline04382(result); err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	fmt.Printf("%s ws-hidden-deadline-04382: status=504 code=request_timeout elapsed=%s upstream_attempts=%d owner=account-a followup=%s\n",
		p.name, elapsed, len(result.attempts), completed)
	return nil
}

func readDeadlineError04382(conn net.Conn, reader *bufio.Reader) (string, int, string, int, error) {
	payloadOpcode, payload, heartbeats, err := readWebSocketLivenessFrame04383(conn, reader)
	if err != nil {
		return "", 0, "", heartbeats, err
	}
	if payloadOpcode != 1 {
		return "", 0, "", heartbeats, fmt.Errorf("expected 504 text frame, got opcode %d", payloadOpcode)
	}
	var event struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &event) != nil {
		return "", 0, "", heartbeats, fmt.Errorf("malformed upstream error frame: %s", payload)
	}
	if event.Type != "error" {
		return "", 0, "", heartbeats, fmt.Errorf("hidden precommit frame leaked to client: %s", payload)
	}
	return event.Error.Code, event.Status, event.Error.Message, heartbeats, nil
}

func readDeadlineCompleted04382(conn net.Conn, reader *bufio.Reader) (string, error) {
	opcode, payload, _, err := readWebSocketLivenessFrame04383(conn, reader)
	if err != nil {
		return "", err
	}
	if opcode != 1 {
		return "", fmt.Errorf("expected followup text frame, got opcode %d", opcode)
	}
	var event struct {
		Type     string `json:"type"`
		Response struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	if json.Unmarshal(payload, &event) != nil {
		return "", fmt.Errorf("malformed followup: %s", payload)
	}
	if event.Type == "response.completed" {
		return event.Response.ID, nil
	}
	return "", fmt.Errorf("local socket had incorrect followup: %s", payload)
}

func readDeadlineFrame04382(reader *bufio.Reader) ([]byte, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return nil, err
	}
	payload, err := frame.ReadPayload(reader, 1<<20)
	if err != nil {
		return nil, err
	}
	frame.Unmask(payload)
	if frame.Opcode != 1 {
		return nil, fmt.Errorf("unexpected WebSocket frame opcode %d", frame.Opcode)
	}
	if len(payload) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return payload, nil
}
