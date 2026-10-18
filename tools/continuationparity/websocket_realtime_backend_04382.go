package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const realtimeUpdate04382 = `{"type":"session.update","session":{"instructions":"synthetic realtime"}}`
const realtimeTurn04382 = `{"type":"response.create","response":{"instructions":"synthetic turn"}}`
const realtimeAudio04382 = `{"type":"input_audio_buffer.append","audio":"AAEC"}`

type realtimeBackend04382 struct {
	wireChanged                                        bool
	backend                                            *backend
	mu                                                 sync.Mutex
	writer                                             sync.Mutex
	live                                               net.Conn
	owner                                              string
	probes, handshakes, controls, audio, turns, closed int
	closedSignal                                       chan struct{}
}

func newRealtimeBackend04382() *realtimeBackend04382 {
	r := &realtimeBackend04382{backend: &backend{wsCounts: make(map[string]int)}, closedSignal: make(chan struct{}, 4)}
	r.backend.server = httptest.NewServer(http.HandlerFunc(r.serve))
	return r
}

func (r *realtimeBackend04382) serve(w http.ResponseWriter, request *http.Request) {
	if strings.HasSuffix(request.URL.Path, "/wham/usage") {
		r.mu.Lock()
		r.probes++
		r.mu.Unlock()
		weekly := 100
		if request.Header.Get("ChatGPT-Account-Id") == "account-a" {
			weekly = 10
		}
		writeJSON(w, 200, fmt.Sprintf(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":10,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":%d,"reset_at":4102444800,"limit_window_seconds":604800}}}`, weekly))
		return
	}
	if strings.HasSuffix(request.URL.Path, "/responses") {
		r.mu.Lock()
		r.wireChanged = r.wireChanged || request.Header.Get("Authorization") != "Bearer token-a"
		r.mu.Unlock()
		r.backend.serveWebSocket(w, request)
		return
	}
	if !strings.HasSuffix(request.URL.Path, "/live") {
		http.Error(w, "unexpected synthetic route", 404)
		return
	}
	conn, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	defer func() {
		r.mu.Lock()
		r.closed++
		r.mu.Unlock()
		r.closedSignal <- struct{}{}
	}()
	digest := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
	if buffered.Flush() != nil {
		return
	}
	r.mu.Lock()
	r.live, r.owner = conn, request.Header.Get("ChatGPT-Account-Id")
	r.wireChanged = r.wireChanged || request.Header.Get("Authorization") != "Bearer token-a"
	r.handshakes++
	r.mu.Unlock()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
	for {
		frame, payload, err := readRealtimeFrame04382(buffered.Reader)
		if err != nil {
			return
		}
		r.mu.Lock()
		r.wireChanged = r.wireChanged || !frame.Masked()
		r.mu.Unlock()
		if frame.Opcode == 8 {
			_ = r.write(8, payload)
			return
		}
		if frame.Opcode == 9 {
			_ = r.write(10, payload)
			continue
		}
		if frame.Opcode == 2 {
			r.mu.Lock()
			r.audio++
			r.mu.Unlock()
			if r.write(2, payload) != nil {
				return
			}
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if frame.Opcode != 1 || json.Unmarshal(payload, &event) != nil {
			return
		}
		var reply string
		r.mu.Lock()
		switch event.Type {
		case "session.update":
			r.controls++
			r.wireChanged = r.wireChanged || string(payload) != realtimeUpdate04382
			reply = `{"type":"session.updated","session":{"id":"synthetic-live"}}`
		case "input_audio_buffer.append":
			r.controls++
			r.wireChanged = r.wireChanged || string(payload) != realtimeAudio04382
			reply = `{"type":"output_audio.delta","delta":"AAEC"}`
		case "response.create":
			r.turns++
			r.wireChanged = r.wireChanged || string(payload) != realtimeTurn04382
			reply = `{"type":"response.completed","response":{"id":"resp-live"}}`
		}
		r.mu.Unlock()
		if reply == "" || r.write(1, []byte(reply)) != nil {
			return
		}
	}
}

func (r *realtimeBackend04382) write(opcode byte, payload []byte) error {
	r.mu.Lock()
	conn := r.live
	r.mu.Unlock()
	r.writer.Lock()
	defer r.writer.Unlock()
	return websocketframe.WriteFrame(conn, opcode, payload, false)
}

func readRealtimeFrame04382(reader *bufio.Reader) (websocketframe.Frame, []byte, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return frame, nil, err
	}
	payload, err := frame.ReadPayload(reader, 1<<20)
	frame.Unmask(payload)
	return frame, payload, err
}
