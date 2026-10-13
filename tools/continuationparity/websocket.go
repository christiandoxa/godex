package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func (b *backend) serveWebSocket(writer http.ResponseWriter, request *http.Request) {
	key := request.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(writer, "missing client key", http.StatusBadRequest)
		return
	}
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	connection, buffered, err := writer.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer connection.Close()
	_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		base64.StdEncoding.EncodeToString(digest[:]))
	if buffered.Flush() != nil {
		return
	}
	// The synthetic backend is deliberately deterministic and loopback-only.
	for {
		_ = connection.SetReadDeadline(time.Now().Add(12 * time.Second))
		frame, err := websocketframe.ReadHeader(buffered.Reader)
		if err != nil {
			return
		}
		data, err := frame.ReadPayload(buffered.Reader, 1<<20)
		if err != nil {
			return
		}
		frame.Unmask(data)
		if frame.Opcode == 8 {
			return
		}
		if frame.Opcode == 9 {
			_ = websocketframe.WriteFrame(connection, 10, data, false)
			continue
		}
		if frame.Opcode != 1 {
			return
		}
		var envelope struct {
			Type string `json:"type"`
			Mode string `json:"mode"`
		}
		if json.Unmarshal(data, &envelope) != nil || envelope.Type != "response.create" {
			_ = websocketframe.WriteFrame(connection, 1, []byte(`{"type":"error","status":400,"error":{"code":"invalid_request_error"}}`), false)
			return
		}
		account := request.Header.Get("ChatGPT-Account-Id")
		b.mu.Lock()
		b.attempts = append(b.attempts, attempt{account: account, mode: envelope.Mode, body: string(data)})
		b.wsCounts[envelope.Mode]++
		seen := b.wsCounts[envelope.Mode]
		b.mu.Unlock()
		var payloads []string
		switch envelope.Mode {
		case "ws-keepalive":
			if seen == 1 {
				payloads = []string{
					`{"type":"response.created","response":{"id":"discarded-response"}}`,
					`{"type":"keepalive"}`,
					`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"synthetic capacity error"}}}`,
				}
			} else {
				payloads = []string{
					`{"type":"response.output_text.delta","delta":"recovered"}`,
					`{"type":"response.completed","response":{"id":"resp-recovered"}}`,
				}
			}
		case "ws-committed":
			payloads = []string{
				`{"type":"keepalive"}`,
				`{"type":"response.output_text.delta","delta":"visible"}`,
				`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`,
			}
		case "ws-bound", "ws-bound-restart":
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-bound"}}`}
		default:
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-default"}}`}
		}
		for _, payload := range payloads {
			if websocketframe.WriteFrame(connection, 1, []byte(payload), false) != nil {
				return
			}
		}
	}
}

type wsProbe struct {
	types []string
	delta []string
	body  []string
}

func wsConnect(endpoint string) (net.Conn, *bufio.Reader, error) {
	endpointURL, err := url.Parse(endpoint + "/responses")
	if err != nil {
		return nil, nil, err
	}
	connection, err := net.DialTimeout("tcp", endpointURL.Host, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReader(connection)
	_, err = fmt.Fprintf(connection,
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Protocol: codex.realtime\r\n\r\n",
		endpointURL.EscapedPath(), endpointURL.Host)
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		_ = connection.Close()
		return nil, nil, err
	}
	if !strings.HasPrefix(line, "HTTP/1.1 101 ") {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("websocket rejected with %q", strings.TrimSpace(line))
	}
	for {
		header, err := reader.ReadString('\n')
		if err != nil {
			_ = connection.Close()
			return nil, nil, err
		}
		if strings.TrimSpace(header) == "" {
			break
		}
	}
	return connection, reader, nil
}

func websocketProbe(endpoint, mode string) (wsProbe, error) {
	connection, reader, err := wsConnect(endpoint)
	if err != nil {
		return wsProbe{}, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(28 * time.Second))
	request := map[string]any{"type": "response.create", "mode": mode, "input": []any{}}
	if mode == "ws-bound" || mode == "ws-bound-restart" {
		// The verified previous response belongs to the primary profile;
		// the soft session key is deliberately bound to the other profile.
		request["previous_response_id"] = "resp-main"
		request["session_id"] = "sess-conflict"
	}
	body, err := json.Marshal(request)
	if err != nil {
		return wsProbe{}, err
	}
	if err := websocketframe.WriteFrame(connection, 1, body, true); err != nil {
		return wsProbe{}, err
	}
	result := wsProbe{}
	for len(result.types) < 10 {
		frame, err := websocketframe.ReadHeader(reader)
		if err != nil {
			return result, fmt.Errorf("read %s: %w (events=%v)", mode, err, result.types)
		}
		payload, err := frame.ReadPayload(reader, 1<<20)
		if err != nil {
			return result, err
		}
		frame.Unmask(payload)
		if frame.Opcode == 9 {
			_ = websocketframe.WriteFrame(connection, 10, payload, true)
			continue
		}
		if frame.Opcode != 1 {
			return result, fmt.Errorf("unexpected opcode %d", frame.Opcode)
		}
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if json.Unmarshal(payload, &event) != nil {
			return result, fmt.Errorf("malformed websocket payload %q", payload)
		}
		result.types = append(result.types, event.Type)
		result.delta = append(result.delta, event.Delta)
		result.body = append(result.body, string(payload))
		if event.Type == "response.failed" || event.Type == "response.completed" || event.Type == "error" {
			return result, nil
		}
	}
	return result, errors.New("too many websocket events without terminal")
}

func runWebSocketProduct(root string, b *backend, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	b.mu.Lock()
	b.wsCounts = make(map[string]int)
	b.mu.Unlock()
	endpoint, cmd, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer func() { stopGateway(cmd) }()
	start := len(mustAttempts(b))
	// Both products must discard precommit metadata/heartbeats on a
	// retryable failure, and must never replay already-visible output. Prodex
	// may recover on the same credential instead of selecting a different one.
	probes := []websocketExpectation{
		{"ws-keepalive", []string{"response.output_text.delta", "response.completed"}, "recovered", 2},
		{"ws-committed", []string{"keepalive", "response.output_text.delta", "response.failed"}, "visible", 1},
		{"ws-bound", []string{"response.completed"}, "", 1},
	}
	for _, probe := range probes {
		before := len(mustAttempts(b))
		got, err := websocketProbe(endpoint, probe.mode)
		if err != nil {
			return fmt.Errorf("%s: %w", probe.mode, err)
		}
		observed := websocketAttemptsForMode(b, before, probe.mode)
		if err := verifyWebsocketExpectation(probe, got, observed); err != nil {
			return err
		}
		if probe.mode == "ws-bound" && observed[0] != "account-a" {
			return fmt.Errorf("%s ignored trusted hard previous-response owner: %v", probe.mode, observed)
		}
		fmt.Printf("%s %s: upstream=%s client=%s\n", p.name, probe.mode, strings.Join(observed, ","),
			strings.Join(got.types, ","))
	}
	// Restart only the local gateway process. The detached WS owner must
	// come from persisted continuation state, not a retained TCP connection
	// or a warm in-memory routing cache.
	stopGateway(cmd)
	endpoint, cmd, err = startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return fmt.Errorf("restart WebSocket gateway: %w", err)
	}
	restart := websocketExpectation{
		mode: "ws-bound-restart", want: []string{"response.completed"}, attempts: 1,
	}
	before := len(mustAttempts(b))
	got, err := websocketProbe(endpoint, restart.mode)
	if err != nil {
		return fmt.Errorf("%s: %w", restart.mode, err)
	}
	observed := websocketAttemptsForMode(b, before, restart.mode)
	if err := verifyWebsocketExpectation(restart, got, observed); err != nil {
		return err
	}
	if observed[0] != "account-a" {
		return fmt.Errorf("%s lost verified hard owner after process restart: %v", restart.mode, observed)
	}
	fmt.Printf("%s %s: upstream=%s client=%s\n", p.name, restart.mode,
		strings.Join(observed, ","), strings.Join(got.types, ","))
	if len(mustAttempts(b)[start:]) < 5 {
		return errors.New("websocket fixture recorded insufficient upstream attempts")
	}
	return nil
}

// websocketExpectation is independent of account name selection: Prodex
// 0.437.9 may retry one transiently failed owner before considering rotation.
type websocketExpectation struct {
	mode     string
	want     []string
	delta    string
	attempts int
}

func verifyWebsocketExpectation(probe websocketExpectation, got wsProbe, observed []string) error {
	if len(observed) != probe.attempts {
		return fmt.Errorf("%s upstream attempts=%v, want %d; client=%v",
			probe.mode, observed, probe.attempts, got.body)
	}
	if len(got.types) != len(probe.want) {
		return fmt.Errorf("%s event kinds %v, want %v; frames=%v",
			probe.mode, got.types, probe.want, got.body)
	}
	for i, want := range probe.want {
		if got.types[i] != want {
			return fmt.Errorf("%s event kinds %v, want %v", probe.mode, got.types, probe.want)
		}
	}
	if !strings.Contains(strings.Join(got.delta, ","), probe.delta) {
		return fmt.Errorf("%s missing visible %q in %v", probe.mode, probe.delta, got.body)
	}
	for _, body := range got.body {
		if strings.Contains(body, "discarded-response") {
			return fmt.Errorf("%s leaked failed upstream response ID in %v", probe.mode, got.body)
		}
	}
	return nil
}

func websocketAttemptsForMode(b *backend, from int, mode string) []string {
	recorded := mustAttempts(b)[from:]
	observed := make([]string, 0, len(recorded))
	for _, item := range recorded {
		if item.mode == mode {
			observed = append(observed, item.account)
		}
	}
	return observed
}
