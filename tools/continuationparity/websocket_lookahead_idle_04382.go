package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const lookaheadIdleMode04382 = "ws-lookahead-idle-04382"
const lookaheadIdleBody04382 = `{"type":"response.create","mode":"ws-lookahead-idle-04382","input":[]}`
const lookaheadPadding04382 = 8192

type websocketLookaheadIdleResult04382 struct {
	eventType, padding string
	closeOpcode        byte
	heartbeats         int
	closePayload       []byte
	elapsed            time.Duration
	attempts           []attempt
	handshakes         int
}

func verifyWebSocketLookaheadIdle04382(result websocketLookaheadIdleResult04382) error {
	if result.eventType != "response.in_progress" || result.padding != strings.Repeat("x", lookaheadPadding04382) {
		return fmt.Errorf("lookahead hold content or event kind was changed/lost: kind=%q padding_bytes=%d", result.eventType, len(result.padding))
	}
	if result.heartbeats < 1 || result.heartbeats > 3 {
		return fmt.Errorf("0.439.0 missing/excess client Ping heartbeat: %d", result.heartbeats)
	}
	if result.closeOpcode != 8 || len(result.closePayload) != 0 {
		return fmt.Errorf("lookahead committed idle must close WebSocket gracefully: opcode=%d payload=%q", result.closeOpcode, result.closePayload)
	}
	if result.elapsed < 1500*time.Millisecond || result.elapsed > 4000*time.Millisecond {
		return fmt.Errorf("lookahead promotion retained precommit timer instead of 2000ms committed idle: elapsed=%s", result.elapsed)
	}
	if result.handshakes != 1 || len(result.attempts) != 1 {
		return fmt.Errorf("lookahead promotion retried or reconnected: attempts=%d handshakes=%d", len(result.attempts), result.handshakes)
	}
	observed := result.attempts[0]
	if observed.account != "account-a" || observed.mode != lookaheadIdleMode04382 || observed.body != lookaheadIdleBody04382 {
		return fmt.Errorf("lookahead promotion changed owner/opaque request: account=%q mode=%q body=%q", observed.account, observed.mode, observed.body)
	}
	return nil
}

// This frame is intentionally precommit-only until it exceeds 8KiB. When
// promoted the SAME commit.rs clock owner applies as for visible output.
// The upstream stays open 7 seconds to distinguish configured 2-second
// committed idle from Godex's original 8-second precommit progress timeout.
func serveLookaheadStalledStream04382(conn net.Conn) {
	payload := []byte(`{"type":"response.in_progress","padding":"` + strings.Repeat("x", lookaheadPadding04382) + `"}`)
	if websocketframe.WriteFrame(conn, 1, payload, false) != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(7 * time.Second))
	_, _ = conn.Read(make([]byte, 1))
}

func runWebSocketLookaheadIdleProduct04382(root string, b *backend, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	env := make([]string, 0, len(p.env)+2)
	for _, value := range p.env {
		if strings.HasPrefix(value, "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=") || strings.HasPrefix(value, "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=") {
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
	if err := websocketframe.WriteFrame(conn, 1, []byte(lookaheadIdleBody04382), true); err != nil {
		return err
	}
	payload, err := readDeadlineFrame04382(reader)
	if err != nil {
		return fmt.Errorf("%s: promoted 8KiB precommit frame: %w", p.name, err)
	}
	var event struct{ Type, Padding string }
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("%s: decode promoted hold: %w", p.name, err)
	}
	began := time.Now()
	closeOpcode, closePayload, heartbeatCount, err := readWebSocketLivenessFrame04383(conn, reader)
	if err != nil {
		return fmt.Errorf("%s: committed idle close/ping: %w", p.name, err)
	}
	b.mu.Lock()
	observed := websocketLookaheadIdleResult04382{
		eventType: event.Type, padding: event.Padding, closeOpcode: closeOpcode, heartbeats: heartbeatCount,
		closePayload: closePayload, elapsed: time.Since(began),
		handshakes: b.wsHandshakes - initialHandshakes,
		attempts:   append([]attempt(nil), b.attempts[initialAttempts:]...),
	}
	b.mu.Unlock()
	if err := verifyWebSocketLookaheadIdle04382(observed); err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	fmt.Printf("%s ws-lookahead-idle-04382: padding=%d bytes promoted, empty close opcode=8 after %s, one upstream attempt/account-a\n", p.name, len(observed.padding), observed.elapsed)
	return nil
}
