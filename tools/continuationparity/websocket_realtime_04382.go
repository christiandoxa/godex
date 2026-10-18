package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

func prepareRealtimeProduct04382(root string, p *product) error {
	if err := prepareProduct(root, p); err != nil {
		return err
	}
	if err := writeAuth(filepath.Join(p.user, ".codex", "auth.json"), "account-c", "synthetic-c"); err != nil {
		return err
	}
	if _, err := invoke(p.binary, p.env, "profile", "import-current", "third"); err != nil {
		return err
	}
	if _, err := invoke(p.binary, p.env, "profile", "use", "--profile", "main"); err != nil {
		return err
	}
	if err := seedProductState(*p); err != nil {
		return err
	}
	path := filepath.Join(p.state, "runtime-usage-snapshots.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	values := snapshot["value"].(map[string]any)
	ready := "main"
	if p.name == "godex" {
		var state struct{ Accounts []struct{ ID, Name string } }
		data, err := os.ReadFile(filepath.Join(p.state, "state.json"))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		for _, account := range state.Accounts {
			if account.Name == "main" {
				ready = account.ID
			}
		}
	} else {
		values["third"] = map[string]any{"checked_at": time.Now().Unix(), "plan_type": "plus", "five_hour_status": "Ready", "five_hour_remaining_percent": 90, "weekly_status": "Exhausted", "weekly_remaining_percent": 0, "weekly_reset_at": 4102444800}
	}
	for key, raw := range values {
		if key != ready {
			value := raw.(map[string]any)
			value["weekly_status"], value["weekly_remaining_percent"], value["weekly_reset_at"] = "Exhausted", 0, 4102444800
		}
	}
	if err := writeJSONFile(path, snapshot); err != nil {
		return err
	}
	if len(values) != 3 || values[ready] == nil {
		return errors.New("realtime fixture requires one ready and two weekly-exhausted profiles")
	}
	env := p.env[:0]
	for _, value := range p.env {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT", "GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT", "PRODEX_RUNTIME_LOG_RECORD":
			continue
		}
		env = append(env, value)
	}
	p.env = append(env, "PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=2", "GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT=2", "PRODEX_RUNTIME_LOG_RECORD=1")
	return nil
}

func runWebSocketRealtimeProduct04382(root string, p product) error {
	for _, closure := range []string{"client", "upstream", "cancel", "shutdown"} {
		if err := runRealtimeClosure04382(filepath.Join(root, closure), p, closure); err != nil {
			return fmt.Errorf("%s close: %w", closure, err)
		}
	}
	return nil
}

func runRealtimeClosure04382(root string, p product, closure string) error {
	if err := prepareRealtimeProduct04382(root, &p); err != nil {
		return err
	}
	r := newRealtimeBackend04382()
	defer r.backend.close()
	endpoint, process, err := startGateway(p, r.backend.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer func() {
		if process != nil {
			stopGateway(process)
		}
	}()
	liveEndpoint, err := realtimeEndpoint04382(p, endpoint)
	if err != nil {
		return err
	}
	// Prepare masking before the sidecar starts its short post-upgrade pump.
	preparedAt := time.Now()
	var initial bytes.Buffer
	if err := websocketframe.WriteFrame(&initial, 1, []byte(realtimeUpdate04382), true); err != nil {
		return err
	}
	preparedIn := time.Since(preparedAt)
	live, reader, err := wsConnectURL(liveEndpoint)
	if err != nil {
		return err
	}
	defer live.Close()
	_ = live.SetDeadline(time.Now().Add(10 * time.Second))
	sentAt := time.Now()
	if _, err := initial.WriteTo(live); err != nil {
		return err
	}
	sentIn := time.Since(sentAt)
	if err := readRealtimeReply04382(reader, 1, `{"type":"session.updated","session":{"id":"synthetic-live"}}`); err != nil {
		return fmt.Errorf("session update (prepare=%s write=%s): %w", preparedIn, sentIn, err)
	}
	if err := realtimeRoundTrip04382(live, reader, 1, []byte(realtimeAudio04382), `{"type":"output_audio.delta","delta":"AAEC"}`); err != nil {
		return fmt.Errorf("audio control: %w", err)
	}
	if err := realtimeRoundTrip04382(live, reader, 2, []byte{0, 1, 2}, string([]byte{0, 1, 2})); err != nil {
		return fmt.Errorf("binary audio: %w", err)
	}
	if err := realtimeRoundTrip04382(live, reader, 1, []byte(realtimeTurn04382), `{"type":"response.completed","response":{"id":"resp-live"}}`); err != nil {
		return fmt.Errorf("live turn: %w", err)
	}
	follower, followerReader, err := wsConnect(endpoint)
	if err != nil {
		return err
	}
	defer follower.Close()
	_ = follower.SetDeadline(time.Now().Add(10 * time.Second))
	type completion struct {
		id  string
		err error
	}
	done := make(chan completion, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		id, err := capacityWebSocketTurn04382(follower, followerReader)
		done <- completion{id, err}
	}()
	defer func() { _ = follower.Close(); <-finished }()
	if err := waitRealtimeSaturation04382(p); err != nil {
		return err
	}
	r.backend.mu.Lock()
	premature := len(r.backend.attempts) != 0 || r.backend.wsHandshakes != 0
	r.backend.mu.Unlock()
	if premature {
		return errors.New("waiting Responses dispatched before realtime close")
	}
	switch closure {
	case "client":
		err = websocketframe.WriteFrame(live, 8, []byte{3, 232}, true)
	case "upstream":
		err = r.write(8, []byte{3, 232})
	case "cancel":
		err = live.Close()
	case "shutdown":
		stopGateway(process)
		if process.ProcessState == nil || !process.ProcessState.Success() {
			process = nil
			return errors.New("gateway did not shut down gracefully")
		}
		process = nil
	}
	if err != nil {
		return err
	}
	if closure == "client" || closure == "upstream" {
		frame, payload, err := readRealtimeFrame04382(reader)
		if err != nil {
			return fmt.Errorf("close acknowledgement: %w", err)
		}
		if frame.Opcode != 8 || !bytes.Equal(payload, []byte{3, 232}) {
			return fmt.Errorf("unexpected close acknowledgement: %d/%v", frame.Opcode, payload)
		}
	}
	select {
	case <-r.closedSignal:
	case <-time.After(5 * time.Second):
		return errors.New("upstream realtime session did not close")
	}
	select {
	case result := <-done:
		if closure == "shutdown" {
			if result.err == nil {
				return errors.New("shutdown dispatched waiting request")
			}
		} else if result.err != nil || result.id != "resp-default" {
			return fmt.Errorf("waiting request completion %q: %w", result.id, result.err)
		}
	case <-time.After(5 * time.Second):
		return errors.New("close did not release permit to waiting request")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backend.mu.Lock()
	defer r.backend.mu.Unlock()
	result := realtimeResult04382{
		owner: r.owner, handshakes: r.handshakes, controls: r.controls,
		audio: r.audio, turns: r.turns, closed: r.closed, waiting: true,
		responseHandshakes: r.backend.wsHandshakes, attempts: r.backend.attempts,
		wireChanged: r.wireChanged,
	}
	if err := verifyRealtimeResult04382(result, closure); err != nil {
		return err
	}
	fmt.Printf("%s realtime %s: quota_probes=%d live_handshakes=1 control=2 audio=1 model_turns=1 close=1 Responses_handshakes=%d waiting_turns=%d owner=account-a\n", p.name, closure, r.probes, result.responseHandshakes, len(result.attempts))
	return nil
}

type realtimeResult04382 struct {
	wireChanged                                bool
	owner                                      string
	handshakes, controls, audio, turns, closed int
	waiting                                    bool
	responseHandshakes                         int
	attempts                                   []attempt
}

func verifyRealtimeResult04382(result realtimeResult04382, closure string) error {
	if result.owner != "account-a" || result.handshakes != 1 || result.controls != 2 || result.audio != 1 || result.turns != 1 || result.closed != 1 || !result.waiting || result.wireChanged {
		return fmt.Errorf("realtime owner/admission/traffic/close mutated: %+v", result)
	}
	want := 1
	if closure == "shutdown" {
		want = 0
	}
	if result.responseHandshakes != want || len(result.attempts) != want {
		return fmt.Errorf("waiting Responses handshake/dispatch mutated: %d/%d", result.responseHandshakes, len(result.attempts))
	}
	if want == 1 && (result.attempts[0].account != "account-a" || result.attempts[0].body != capacityRequestBody04382) {
		return errors.New("waiting Responses owner/request mutated")
	}
	return nil
}

func realtimeEndpoint04382(p product, endpoint string) (string, error) {
	if p.name != "prodex" {
		return endpoint + "/live", nil
	}
	paths, err := filepath.Glob(filepath.Join(p.state, "runtime-broker-live-runtime-*.json"))
	if err != nil {
		return "", err
	}
	if len(paths) != 1 {
		return "", fmt.Errorf("expected one live runtime registry, found %d", len(paths))
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return "", err
	}
	var registry struct {
		Address string `json:"realtime_ws_addr"`
		Mount   string `json:"openai_mount_path"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return "", err
	}
	address, err := net.ResolveTCPAddr("tcp", registry.Address)
	if err != nil || address.IP == nil || !address.IP.IsLoopback() {
		return "", errors.New("realtime sidecar must have a loopback address")
	}
	return "http://" + address.String() + registry.Mount + "/live", nil
}

func realtimeRoundTrip04382(conn io.Writer, reader *bufio.Reader, opcode byte, payload []byte, expected string) error {
	// Avoid splitting a small control frame across the sidecar's 20ms pump.
	writer := bufio.NewWriter(conn)
	if err := websocketframe.WriteFrame(writer, opcode, payload, true); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return readRealtimeReply04382(reader, opcode, expected)
}

func readRealtimeReply04382(reader *bufio.Reader, opcode byte, expected string) error {
	started := time.Now()
	for i := 0; i < 4; i++ {
		frame, data, err := readRealtimeFrame04382(reader)
		if err != nil {
			return fmt.Errorf("read realtime reply after %s: %w", time.Since(started), err)
		}
		if frame.Opcode == 10 {
			continue
		}
		if frame.Masked() || frame.Opcode != opcode || string(data) != expected {
			return fmt.Errorf("unexpected realtime frame opcode=%d payload=%q", frame.Opcode, data)
		}
		return nil
	}
	return errors.New("missing realtime data frame")
}

func waitRealtimeSaturation04382(p product) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		found := false
		err := filepath.WalkDir(filepath.Join(p.state, "logs"), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".log", ".jsonl":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil && bytes.Contains(data, []byte("profile_inflight_saturated")) {
				found = true
			}
			return err
		})
		if found {
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			return errors.New("waiting request never recorded profile inflight saturation")
		}
	}
}
