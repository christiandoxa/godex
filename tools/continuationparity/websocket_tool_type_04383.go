package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func websocketToolTypeMessage04383(shape string) string {
	kind := "function_call_output"
	switch shape {
	case "trailing-space":
		kind += " "
	case "trailing-tab":
		kind += `\t`
	case "leading-space":
		kind = " " + kind
	}
	return `{"type":"response.create","model":"gpt-6","previous_response_id":"resp-main","input":[{"type":"` + kind + `","call_id":"call-tool-suffix","output":"done"}]}`
}

func websocketToolTypeAttempts04383(shape string) int {
	switch shape {
	case "trailing-space", "trailing-tab":
		return 1
	default:
		return 4
	}
}

func runWebSocketToolTypeProduct04383(root string, p product) error {
	for _, shape := range []string{"canonical", "trailing-space", "trailing-tab", "leading-space"} {
		if err := runWebSocketToolTypeCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("WebSocket tool-type %s: %w", shape, err)
		}
	}
	return nil
}

func runWebSocketToolTypeCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "tool-suffix-"+shape); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	backend := newTransportBackend04383("tool-suffix-" + shape)
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
			return fmt.Errorf("WebSocket tool suffix warmup %s: status=%d err=%v", previous, warmup.status, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, backend) {
		if time.Now().After(deadline) {
			return errors.New("WebSocket tool suffix fixture lacks authenticated Ready evidence on two profiles")
		}
		time.Sleep(10 * time.Millisecond)
	}
	connection, reader, err := wsConnectHeaders04383(endpoint+"/responses", "")
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(9 * time.Second))
	body := websocketToolTypeMessage04383(shape)
	if err := websocketframe.WriteFrame(connection, 1, []byte(body), true); err != nil {
		return err
	}
	var frames []string
	for len(frames) < 2 {
		opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
		if err != nil {
			if len(frames) > 0 {
				break
			}
			return fmt.Errorf("WebSocket tool suffix client: %w", err)
		}
		if opcode != 1 {
			return fmt.Errorf("WebSocket tool suffix opcode %d frames=%q", opcode, frames)
		}
		frames = append(frames, string(payload))
		if strings.Contains(string(payload), `"stale_continuation"`) || strings.Contains(string(payload), `"previous_response_not_found"`) {
			break
		}
	}
	backend.mu.Lock()
	attempts := append([]transportAttempt04383(nil), backend.attempts...)
	handshakes := append([]string(nil), backend.handshakes...)
	invalid := backend.invalid
	backend.mu.Unlock()
	if err := verifyWebSocketToolType04383(shape, body, handshakes, attempts, frames, invalid); err != nil {
		return err
	}
	original, err := readHTTPResponseOwner04383(p, "resp-main")
	if err != nil || original != "main" {
		return fmt.Errorf("WebSocket suffix lost original verified response owner %q err=%v", original, err)
	}
	fmt.Printf("%s WebSocket tool type %s: two Ready profiles; %d exact owner attempts; terminal=%q; foreign state untouched\n", p.name, shape, len(attempts), frames)
	return nil
}

func verifyWebSocketToolType04383(shape, body string, handshakes []string, attempts []transportAttempt04383, frames []string, invalid bool) error {
	want := websocketToolTypeAttempts04383(shape)
	if invalid || len(attempts) != want || len(handshakes) != want || len(frames) == 0 {
		return fmt.Errorf("WebSocket suffix wrong attempt count: shape=%s want=%d invalid=%t handshakes=%v attempts=%+v frames=%v", shape, want, invalid, handshakes, attempts, frames)
	}
	if !reflect.DeepEqual(handshakes, []string{"account-a", "account-a", "account-a", "account-a"}[:want]) {
		return fmt.Errorf("WebSocket suffix escaped previous owner: %v", handshakes)
	}
	for _, attempt := range attempts {
		if attempt.owner != "account-a" || !attempt.ready || attempt.body != body || attempt.turn != "" {
			return fmt.Errorf("WebSocket suffix changed attempt/Ready/opaque request: %+v", attempt)
		}
	}
	if !strings.Contains(strings.Join(frames, "\n"), `"stale_continuation"`) &&
		!strings.Contains(strings.Join(frames, "\n"), `"previous_response_not_found"`) {
		return fmt.Errorf("WebSocket suffix did not yield previous-response terminal: %q", frames)
	}
	return nil
}
