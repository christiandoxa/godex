package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const committedIdleMode04382 = "ws-committed-idle-04382"
const committedIdleBody04382 = `{"type":"response.create","mode":"ws-committed-idle-04382","input":[]}`

type websocketCommittedIdleResult04382 struct {
	eventType, delta string
	closeOpcode      byte
	heartbeats       int
	closePayload     []byte
	elapsed          time.Duration
	attempts         []attempt
	handshakes       int
}

func verifyWebSocketCommittedIdle04382(result websocketCommittedIdleResult04382) error {
	if result.eventType != "response.output_text.delta" || result.delta != "visible-idle" {
		return fmt.Errorf("committed WebSocket delta was mutated/lost: %q/%q", result.eventType, result.delta)
	}
	if result.heartbeats < 1 || result.heartbeats > 3 {
		return fmt.Errorf("0.439.0 missing/excess client Ping heartbeat: %d", result.heartbeats)
	}
	if result.closeOpcode != 8 || len(result.closePayload) != 0 {
		return fmt.Errorf("committed WebSocket stream was not closed gracefully: opcode=%d body=%q", result.closeOpcode, result.closePayload)
	}
	// 0.438.2 uses the configured 2000ms stream idle timeout after visible
	// output. An early precommit expiry or old 300s constant must both fail.
	if result.elapsed < 1500*time.Millisecond || result.elapsed > 4000*time.Millisecond {
		return fmt.Errorf("committed WebSocket expiry shifted from configured 2000ms: %s", result.elapsed)
	}
	if result.handshakes != 1 || len(result.attempts) != 1 {
		return fmt.Errorf("committed WebSocket replayed/reconnected: handshakes=%d upstream_attempts=%d", result.handshakes, len(result.attempts))
	}
	observed := result.attempts[0]
	if observed.account != "account-a" || observed.mode != committedIdleMode04382 || observed.body != committedIdleBody04382 {
		return fmt.Errorf("committed WebSocket changed owner/request: account=%q mode=%q body=%q", observed.account, observed.mode, observed.body)
	}
	return nil
}

// Send one real committed model delta, then hold the upstream open well past
// its configured idle timeout. The published Prodex 0.438.2 binary emits a
// graceful EMPTY WebSocket close control frame to the client at about 2s.
// Abrupt TCP EOF or response.completed is NOT wire-identical behavior.
func serveCommittedStalledStream04382(conn net.Conn) {
	_ = websocketframe.WriteFrame(conn, 1, []byte(`{"type":"response.output_text.delta","delta":"visible-idle"}`), false)
	_ = conn.SetReadDeadline(time.Now().Add(7 * time.Second))
	_, _ = conn.Read(make([]byte, 1))
}

func runWebSocketCommittedIdleProduct04382(root string, b *backend, p product) error {
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
	p.env = append(env, "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000", "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000")
	endpoint, command, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stopGateway(command)
	b.mu.Lock()
	initialAttempts := len(b.attempts)
	initialHandshakes := b.wsHandshakes
	b.mu.Unlock()
	conn, reader, err := wsConnect(endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(7 * time.Second))
	if err := websocketframe.WriteFrame(conn, 1, []byte(committedIdleBody04382), true); err != nil {
		return err
	}
	first, err := readDeadlineFrame04382(reader)
	if err != nil {
		return fmt.Errorf("%s: committed model delta: %w", p.name, err)
	}
	var event struct{ Type, Delta string }
	if err := json.Unmarshal(first, &event); err != nil {
		return fmt.Errorf("%s: decode committed frame: %w", p.name, err)
	}
	start := time.Now()
	closeOpcode, closePayload, heartbeatCount, err := readWebSocketLivenessFrame04383(conn, reader)
	if err != nil {
		return fmt.Errorf("%s: committed idle close/ping: %w", p.name, err)
	}
	b.mu.Lock()
	observation := websocketCommittedIdleResult04382{
		eventType: event.Type, delta: event.Delta,
		closeOpcode: closeOpcode, closePayload: closePayload, heartbeats: heartbeatCount,
		elapsed:    time.Since(start),
		attempts:   append([]attempt(nil), b.attempts[initialAttempts:]...),
		handshakes: b.wsHandshakes - initialHandshakes,
	}
	b.mu.Unlock()
	if err := verifyWebSocketCommittedIdle04382(observation); err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	fmt.Printf("%s ws-committed-idle-04382: delta=%q graceful_close_opcode=8 idle=%s upstream_attempts=1 handshakes=1 owner=account-a\n", p.name, event.Delta, observation.elapsed)
	return nil
}
