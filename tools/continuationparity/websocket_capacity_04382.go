package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const capacityMode04382 = "ws-idle-capacity-04382"

const capacityRequestBody04382 = `{"type":"response.create","mode":"ws-idle-capacity-04382","input":[]}`

// Capture the entire independent upstream observation so mutation sentinels
// cannot pass by checking only a final 200 or successful TCP handshake.
type websocketCapacityResult04382 struct {
	handshakes int
	attempts   []attempt
	completed  []string
}

func verifyWebSocketCapacity04382(result websocketCapacityResult04382) error {
	const expectedConnections = 5
	const expectedTurns = 6
	if result.handshakes != expectedConnections || len(result.attempts) != expectedTurns || len(result.completed) != expectedTurns {
		return fmt.Errorf("idle capacity mutated handshakes/turn count: %d/%d/%d, want %d/%d/%d",
			result.handshakes, len(result.attempts), len(result.completed), expectedConnections, expectedTurns, expectedTurns)
	}
	for i, turn := range result.attempts {
		if turn.account != "account-a" || turn.mode != capacityMode04382 || turn.body != capacityRequestBody04382 {
			return fmt.Errorf("turn %d changed owner, mode or opaque request body: account=%q mode=%q body=%q", i, turn.account, turn.mode, turn.body)
		}
		if result.completed[i] != "resp-default" {
			return fmt.Errorf("turn %d lost exact completed response ID: %q", i, result.completed[i])
		}
	}
	return nil
}

// This release-specific two-executable fixture must NOT reuse the sequential
// 58-attempt WebSocket suite as proof of idle permit release. Multiple local
// sockets stay open after their completed response while a fresh session
// still dispatches on the *same* synthetic owner. Reuse of the first local
// socket must use the cached upstream transport without a new handshake.
func runWebSocketCapacityProduct04382(root string, b *backend, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	// Prodex exposes this knob; Godex has a canonical alias wired through
	// the launcher. Both products run at weight=2 / hard capacity=2.
	// The parent process can have unrelated local test tuning. Remove only
	// the two overridden keys so process lookup order cannot influence this
	// fixture. Keep the synthetic credential isolation from prepareProduct.
	env := make([]string, 0, len(p.env)+2)
	for _, value := range p.env {
		if strings.HasPrefix(value, "PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=") ||
			strings.HasPrefix(value, "GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=") {
			continue
		}
		env = append(env, value)
	}
	p.env = append(env,
		"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=2",
		"GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=2",
	)
	endpoint, process, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stopGateway(process)
	b.mu.Lock()
	initialAttempts := len(b.attempts)
	initialHandshakes := b.wsHandshakes
	b.mu.Unlock()
	const idleSockets = 4
	completed := make([]string, 0, idleSockets+2)
	clients := make([]net.Conn, 0, idleSockets+1)
	readers := make([]*bufio.Reader, 0, idleSockets+1)
	defer func() {
		for _, conn := range clients {
			_ = conn.Close()
		}
	}()
	for index := 0; index < idleSockets+1; index++ {
		conn, reader, connErr := wsConnect(endpoint)
		if connErr != nil {
			return fmt.Errorf("%s: open idle socket %d: %w", p.name, index, connErr)
		}
		_ = conn.SetDeadline(time.Now().Add(7 * time.Second))
		clients = append(clients, conn)
		readers = append(readers, reader)
		id, turnErr := capacityWebSocketTurn04382(conn, reader)
		if turnErr != nil {
			return fmt.Errorf("%s: completed idle socket %d: %w", p.name, index, turnErr)
		}
		completed = append(completed, id)
		// Never close a completed local socket here: that would mask the
		// fixed 0.438.2 bug where an idle upstream held the last permit.
	}
	b.mu.Lock()
	firstHandshakes := b.wsHandshakes - initialHandshakes
	firstAttempts := append([]attempt(nil), b.attempts[initialAttempts:]...)
	b.mu.Unlock()
	if firstHandshakes != idleSockets+1 || len(firstAttempts) != idleSockets+1 {
		return fmt.Errorf("%s: idle session fanout sent %d upstream requests across %d handshakes, want %d/%d", p.name, len(firstAttempts), firstHandshakes, idleSockets+1, idleSockets+1)
	}
	reusedID, reuseErr := capacityWebSocketTurn04382(clients[0], readers[0])
	if reuseErr != nil {
		return fmt.Errorf("%s: cached socket reuse failed: %w", p.name, reuseErr)
	}
	completed = append(completed, reusedID)
	b.mu.Lock()
	finalHandshakes := b.wsHandshakes - initialHandshakes
	allAttempts := append([]attempt(nil), b.attempts[initialAttempts:]...)
	b.mu.Unlock()
	if finalHandshakes != idleSockets+1 || len(allAttempts) != idleSockets+2 {
		return fmt.Errorf("%s: reused socket lost transport or duplicate-dispatched: handshakes=%d attempts=%d", p.name, finalHandshakes, len(allAttempts))
	}
	result := websocketCapacityResult04382{
		handshakes: finalHandshakes, attempts: allAttempts, completed: completed,
	}
	if err := verifyWebSocketCapacity04382(result); err != nil {
		return fmt.Errorf("%s: %w", p.name, err)
	}
	fmt.Printf("%s ws-idle-capacity-04382: %d completed turns, %d kept upstream handshakes, owner=account-a, first socket reused\n",
		p.name, len(allAttempts), finalHandshakes)
	return nil
}

func capacityWebSocketTurn04382(conn net.Conn, reader *bufio.Reader) (string, error) {
	if err := websocketframe.WriteFrame(conn, 1, []byte(capacityRequestBody04382), true); err != nil {
		return "", err
	}
	for events := 0; events < 8; events++ {
		frame, err := websocketframe.ReadHeader(reader)
		if err != nil {
			return "", err
		}
		payload, err := frame.ReadPayload(reader, 1<<20)
		if err != nil {
			return "", err
		}
		frame.Unmask(payload)
		if frame.Opcode == 9 {
			if err := websocketframe.WriteFrame(conn, 10, payload, true); err != nil {
				return "", err
			}
			continue
		}
		if frame.Opcode != 1 {
			return "", fmt.Errorf("WebSocket capacity unexpected opcode %d", frame.Opcode)
		}
		var event struct {
			Type     string `json:"type"`
			Response struct {
				ID string `json:"id"`
			} `json:"response"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			return "", err
		}
		if event.Type == "response.completed" {
			return event.Response.ID, nil
		}
		if event.Type == "error" || event.Type == "response.failed" {
			return "", fmt.Errorf("WebSocket capacity failure frame: %s", strings.TrimSpace(string(payload)))
		}
	}
	return "", errors.New("WebSocket capacity response never completed")
}
