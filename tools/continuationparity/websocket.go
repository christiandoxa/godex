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
	b.mu.Lock()
	b.wsHandshakes++
	b.mu.Unlock()
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
		if envelope.Mode == deadlineMode04382 {
			serveHiddenPreludeDeadline04382(connection)
			return
		}
		var payloads []string
		switch envelope.Mode {
		case deadlineFollowupMode04382:
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-after-watchdog"}}`}
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
		case "ws-limit-top":
			// The compiled Mojo terminal policy considers the top-level
			// connection limit even when the nested code is unrelated.
			if seen == 1 {
				payloads = []string{`{"type":"error","error":{"code":"unrelated_failure"},"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"}`}
			} else {
				payloads = []string{`{"type":"response.completed","response":{"id":"resp-limit-top-recovered"}}`}
			}
		case "ws-limit-nested":
			if seen == 1 {
				payloads = []string{`{"type":"error","error":{"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"},"code":"unrelated_failure"}`}
			} else {
				payloads = []string{`{"type":"response.completed","response":{"id":"resp-limit-nested-recovered"}}`}
			}
		case "ws-status-fraction":
			payloads = []string{
				`{"type":"error","status":400.5,"status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-fraction"}}`,
			}
		case "ws-status-overflow":
			payloads = []string{
				`{"type":"error","status":65536,"status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-overflow"}}`,
			}
		case "ws-status-high":
			payloads = []string{
				`{"type":"error","status":65535}`,
				`{"type":"response.completed","response":{"id":"should-not-be-forwarded"}}`,
			}
		case "ws-status-priority":
			// A present but invalid `status` must suppress `status_code`
			// fallback. The first error frame is explicitly NON-terminal
			// in 0.438.0; the following completion still has to arrive.
			payloads = []string{
				`{"type":"error","status":"invalid","status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-priority"}}`,
			}
		case "ws-committed":
			payloads = []string{
				`{"type":"keepalive"}`,
				`{"type":"response.output_text.delta","delta":"visible"}`,
				`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`,
			}
		case "ws-bound", "ws-bound-restart", "ws-turn", "ws-turn-restart":
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-bound"}}`}
		case "ws-unknown-missing", "ws-bound-missing":
			payloads = []string{`{"type":"response.failed","status":400,"response":{"error":{"code":"previous_response_not_found","message":"synthetic missing chain"}}}`}
		case "ws-session-seed", "ws-session", "ws-session-restart":
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-session"}}`}
		default:
			payloads = []string{`{"type":"response.completed","response":{"id":"resp-default"}}`}
		}
		for i, payload := range payloads {
			if envelope.Mode == "ws-status-high" && i == 1 {
				time.Sleep(75 * time.Millisecond)
			}
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
	if mode == "ws-bound" || mode == "ws-bound-restart" || mode == "ws-bound-missing" {
		// The verified previous response belongs to the primary profile;
		// the soft session key is deliberately bound to the other profile.
		request["previous_response_id"] = "resp-main"
		request["session_id"] = "sess-conflict"
	}
	if mode == "ws-turn" || mode == "ws-turn-restart" {
		// A hard opaque turn-state without a previous_response_id must
		// also defeat the conflicting soft session preference.
		request["x-codex-turn-state"] = "turn-a"
		request["session_id"] = "sess-conflict"
	}
	if mode == "ws-session-seed" || mode == "ws-session" || mode == "ws-session-restart" {
		request["session_id"] = "sess-conflict"
	}
	if mode == "ws-unknown" {
		request["previous_response_id"] = "resp-absent"
		request["session_id"] = "sess-conflict"
	}
	if mode == "ws-bound-missing" {
		request["input"] = []any{map[string]any{
			"type": "function_call_output", "call_id": "call-bound", "output": "done",
		}}
	}
	if mode == "ws-unknown-missing" {
		request["previous_response_id"] = "resp-tool-missing"
		request["session_id"] = "sess-conflict"
		request["input"] = []any{map[string]any{
			"type": "function_call_output", "call_id": "call-missing", "output": "done",
		}}
	}
	if mode == "ws-unknown-turn" {
		request["x-codex-turn-state"] = "turn-absent"
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
		if event.Type == "error" && mode == "ws-status-high" {
			// The successful u16 terminal error must end the response; a
			// deliberately delayed `response.completed` must NOT leak.
			_ = connection.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
			extra, frameErr := websocketframe.ReadHeader(reader)
			if frameErr == nil && extra.Opcode != 8 {
				return result, fmt.Errorf("terminal high-u16 error leaked followup frame (opcode=%d)", extra.Opcode)
			}
			return result, nil
		}
		nonterminalError := mode == "ws-status-priority" || mode == "ws-status-fraction" || mode == "ws-status-overflow"
		if event.Type == "response.failed" || event.Type == "response.completed" ||
			(event.Type == "error" && !nonterminalError) {
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
	// All owner expectations were observed on the exact pinned Prodex binary.
	// A failed precommit attempt retries account-a, not account-b; neither
	// a response ID nor a soft session is sufficient evidence of rotation.
	probes := []websocketExpectation{
		{mode: "ws-session-seed", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-keepalive", want: []string{"response.output_text.delta", "response.completed"}, delta: "recovered", attempts: 2, owner: "account-a"},
		{mode: "ws-committed", want: []string{"keepalive", "response.output_text.delta", "response.failed"}, delta: "visible", attempts: 1, owner: "account-a"},
		{mode: "ws-limit-top", want: []string{"response.completed"}, attempts: 2, owner: "account-a",
			rawFrames: []string{`{"type":"response.completed","response":{"id":"resp-limit-top-recovered"}}`}},
		{mode: "ws-limit-nested", want: []string{"response.completed"}, attempts: 2, owner: "account-a",
			rawFrames: []string{`{"type":"response.completed","response":{"id":"resp-limit-nested-recovered"}}`}},
		{mode: "ws-status-priority", want: []string{"error", "response.completed"}, attempts: 1, owner: "account-a",
			rawFrames: []string{`{"type":"error","status":"invalid","status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-priority"}}`}},
		{mode: "ws-status-fraction", want: []string{"error", "response.completed"}, attempts: 1, owner: "account-a",
			rawFrames: []string{`{"type":"error","status":400.5,"status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-fraction"}}`}},
		{mode: "ws-status-overflow", want: []string{"error", "response.completed"}, attempts: 1, owner: "account-a",
			rawFrames: []string{`{"type":"error","status":65536,"status_code":400}`,
				`{"type":"response.completed","response":{"id":"resp-status-overflow"}}`}},
		{mode: "ws-status-high", want: []string{"error"}, attempts: 1, owner: "account-a",
			rawFrames: []string{`{"type":"error","status":65535}`}},
		{mode: "ws-bound", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-turn", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-unknown", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-unknown-turn", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-unknown-missing", want: []string{"error"}, attempts: 4, owner: "account-a", errorCode: "stale_continuation", errorStatus: http.StatusConflict},
		{mode: "ws-bound-missing", want: []string{"error"}, attempts: 4, owner: "account-a", errorCode: "stale_continuation", errorStatus: http.StatusConflict},
		{mode: "ws-session", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
	}
	if err := runWebSocketProbes(b, p.name, endpoint, probes); err != nil {
		return err
	}

	// Discard the entire process and all of its local socket/cache state.
	// Durable continuation owners must be recovered by the new gateway.
	stopGateway(cmd)
	endpoint, cmd, err = startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return fmt.Errorf("restart WebSocket gateway: %w", err)
	}
	restart := []websocketExpectation{
		{mode: "ws-bound-restart", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-turn-restart", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
		{mode: "ws-session-restart", want: []string{"response.completed"}, attempts: 1, owner: "account-a"},
	}
	if err := runWebSocketProbes(b, p.name, endpoint, restart); err != nil {
		return err
	}
	if len(mustAttempts(b)[start:]) < 18 {
		return errors.New("websocket fixture recorded insufficient upstream attempts")
	}
	return nil
}

func runWebSocketProbes(b *backend, productName, endpoint string, probes []websocketExpectation) error {
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
		if err := verifyWebsocketOpaqueChain(b, before, probe.mode); err != nil {
			return err
		}
		fmt.Printf("%s %s: upstream=%s client=%s\n", productName, probe.mode,
			strings.Join(observed, ","), strings.Join(got.types, ","))
	}
	return nil
}

// Account selection is part of the pinned source/binary oracle. In this
// deterministic fixture, Prodex retries the *same* owner on overload.
type websocketExpectation struct {
	mode        string
	want        []string
	delta       string
	attempts    int
	owner       string
	errorCode   string
	errorStatus int
	rawFrames   []string
}

func verifyWebsocketExpectation(probe websocketExpectation, got wsProbe, observed []string) error {
	if len(observed) != probe.attempts {
		return fmt.Errorf("%s upstream attempts=%v, want %d; client=%v",
			probe.mode, observed, probe.attempts, got.body)
	}
	if probe.owner != "" {
		for _, account := range observed {
			if account != probe.owner {
				return fmt.Errorf("%s routed to account %q, want canonical owner %q",
					probe.mode, account, probe.owner)
			}
		}
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
	if len(probe.rawFrames) != 0 {
		if len(got.body) != len(probe.rawFrames) {
			return fmt.Errorf("%s missing exact native terminality frames", probe.mode)
		}
		for index, expected := range probe.rawFrames {
			if got.body[index] != expected {
				return fmt.Errorf("%s frame[%d]=%q, expected %q", probe.mode, index, got.body[index], expected)
			}
		}
	}
	if !strings.Contains(strings.Join(got.delta, ","), probe.delta) {
		return fmt.Errorf("%s missing visible %q in %v", probe.mode, probe.delta, got.body)
	}
	if probe.errorCode != "" {
		if len(got.body) == 0 {
			return fmt.Errorf("%s missing expected error code %q", probe.mode, probe.errorCode)
		}
		var envelope struct {
			Status int `json:"status"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
			Response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(got.body[len(got.body)-1]), &envelope); err != nil ||
			(envelope.Error.Code != probe.errorCode && envelope.Response.Error.Code != probe.errorCode) ||
			(probe.errorStatus != 0 && envelope.Status != probe.errorStatus) {
			return fmt.Errorf("%s terminal code/status does not match %q/%d: frames=%v err=%v",
				probe.mode, probe.errorCode, probe.errorStatus, got.body, err)
		}
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
