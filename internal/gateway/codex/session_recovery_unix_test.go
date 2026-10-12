//go:build !windows

package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const nativeRecoveryTestThread = "00000000-0000-7000-8000-000000000042"

type nativeRecoveryScenario struct {
	status            string
	goal              *string
	expectedGoal      bool
	exhaustedBudget   bool
	wrongThread       bool
	wrongSession      bool
	wrongWorkspace    bool
	wrongHome         bool
	rejectMutation    bool
	ambiguousMutation bool
}

func defaultNativeRecoveryScenario() nativeRecoveryScenario {
	goal := "blocked"
	return nativeRecoveryScenario{status: "idle", goal: &goal, expectedGoal: true}
}

func TestNativeRecoveryUsesOwnedThreadGoalMutation(t *testing.T) {
	outcome, calls, released := runNativeRecoveryFixture(t, defaultNativeRecoveryScenario())
	if outcome != NativeRecoveryAccepted || !released {
		t.Fatalf("outcome=%v released=%t", outcome, released)
	}
	writes := nativeRecoveryMutations(calls)
	if len(writes) != 1 || writes[0]["method"] != "thread/goal/set" {
		t.Fatalf("mutations=%v", writes)
	}
	params, _ := writes[0]["params"].(map[string]any)
	if params["threadId"] != nativeRecoveryTestThread || params["status"] != "active" || params["origin"] != "automatic" {
		t.Fatalf("goal mutation params=%v", params)
	}
	for _, call := range calls {
		if call["method"] == "thread/resume" || call["method"] == "/goal resume" {
			t.Fatal("recovery attempted keyboard or frontend resume")
		}
	}
}

func TestNativeRecoveryDefersBusyAndUnloadedThreads(t *testing.T) {
	for _, status := range []string{"active", "notLoaded", "unknown", "futureStatus"} {
		scenario := defaultNativeRecoveryScenario()
		scenario.status = status
		outcome, calls, released := runNativeRecoveryFixture(t, scenario)
		if outcome != NativeRecoveryDeferred || released || len(nativeRecoveryMutations(calls)) != 0 {
			t.Fatalf("status=%s outcome=%v released=%t calls=%v", status, outcome, released, calls)
		}
	}
}

func TestNativeRecoveryStopsUserGoalBudgetAndMissingGoalControls(t *testing.T) {
	for _, goalStatus := range []string{"paused", "budgetLimited", "complete", "unrecognized"} {
		goal := goalStatus
		scenario := defaultNativeRecoveryScenario()
		scenario.goal = &goal
		outcome, calls, released := runNativeRecoveryFixture(t, scenario)
		if outcome != NativeRecoveryStopped || released || len(nativeRecoveryMutations(calls)) != 0 {
			t.Fatalf("goal=%s outcome=%v released=%t calls=%v", goalStatus, outcome, released, calls)
		}
	}
	for _, expectedGoal := range []bool{true, false} {
		scenario := defaultNativeRecoveryScenario()
		scenario.goal = nil
		scenario.expectedGoal = expectedGoal
		outcome, calls, released := runNativeRecoveryFixture(t, scenario)
		if expectedGoal {
			if outcome != NativeRecoveryStopped || released || len(nativeRecoveryMutations(calls)) != 0 {
				t.Fatalf("missing expected goal outcome=%v released=%t calls=%v", outcome, released, calls)
			}
			continue
		}
		if outcome != NativeRecoveryAccepted || !released {
			t.Fatalf("missing non-goal outcome=%v released=%t calls=%v", outcome, released, calls)
		}
		writes := nativeRecoveryMutations(calls)
		if len(writes) != 1 || writes[0]["method"] != "turn/start" {
			t.Fatalf("non-goal mutations=%v", writes)
		}
		params, _ := writes[0]["params"].(map[string]any)
		input, _ := params["input"].([]any)
		text, _ := input[0].(map[string]any)
		if text["text"] != nativeRecoveryPrompt {
			t.Fatalf("continuation prompt=%v", text["text"])
		}
	}
	scenario := defaultNativeRecoveryScenario()
	scenario.exhaustedBudget = true
	outcome, calls, released := runNativeRecoveryFixture(t, scenario)
	if outcome != NativeRecoveryStopped || released || len(nativeRecoveryMutations(calls)) != 0 {
		t.Fatalf("exhausted budget outcome=%v released=%t calls=%v", outcome, released, calls)
	}
}

func TestNativeRecoveryRequiresExactHomeThreadSessionAndWorkspace(t *testing.T) {
	for _, scenario := range []nativeRecoveryScenario{
		func() nativeRecoveryScenario {
			value := defaultNativeRecoveryScenario()
			value.wrongHome = true
			return value
		}(),
		func() nativeRecoveryScenario {
			value := defaultNativeRecoveryScenario()
			value.wrongThread = true
			return value
		}(),
		func() nativeRecoveryScenario {
			value := defaultNativeRecoveryScenario()
			value.wrongSession = true
			return value
		}(),
		func() nativeRecoveryScenario {
			value := defaultNativeRecoveryScenario()
			value.wrongWorkspace = true
			return value
		}(),
	} {
		outcome, calls, released := runNativeRecoveryFixture(t, scenario)
		if outcome != NativeRecoveryStopped || released || len(nativeRecoveryMutations(calls)) != 0 {
			t.Fatalf("foreign identity outcome=%v released=%t calls=%v", outcome, released, calls)
		}
	}
}

func TestNativeRecoveryNeverReplaysAmbiguousMutation(t *testing.T) {
	scenario := defaultNativeRecoveryScenario()
	scenario.ambiguousMutation = true
	outcome, calls, _ := runNativeRecoveryFixture(t, scenario)
	if outcome != NativeRecoveryAmbiguous || len(nativeRecoveryMutations(calls)) != 1 {
		t.Fatalf("ambiguous mutation outcome=%v calls=%v", outcome, calls)
	}
	scenario = defaultNativeRecoveryScenario()
	scenario.rejectMutation = true
	outcome, calls, _ = runNativeRecoveryFixture(t, scenario)
	if outcome != NativeRecoveryStopped || len(nativeRecoveryMutations(calls)) != 1 {
		t.Fatalf("rejected mutation outcome=%v calls=%v", outcome, calls)
	}
}

func TestNativeRecoverySystemErrorUsesSameOwnedThread(t *testing.T) {
	scenario := defaultNativeRecoveryScenario()
	scenario.status = "systemError"
	outcome, calls, released := runNativeRecoveryFixture(t, scenario)
	if outcome != NativeRecoveryAccepted || !released || len(nativeRecoveryMutations(calls)) != 1 {
		t.Fatalf("system error outcome=%v released=%t calls=%v", outcome, released, calls)
	}
}

func TestNativeRecoveryUsesOneDeadlineWithUnsolicitedMessages(t *testing.T) {
	path := nativeRecoveryTestSocket(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		request, readErr := http.ReadRequest(reader)
		if readErr != nil {
			return
		}
		key := request.Header.Get("Sec-WebSocket-Key")
		_, _ = fmt.Fprintf(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", nativeWebSocketAccept(key))
		if _, readErr = readNativeTextFrame(reader); readErr != nil {
			return
		}
		_ = connection.SetWriteDeadline(time.Now().Add(time.Second))
		for {
			if writeErr := websocketframe.WriteFrame(connection, 1, []byte(`{"method":"unrelated","params":{}}`), false); writeErr != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	defer func() { _ = listener.Close(); <-serverDone }()
	socket, err := connectNativeRecoverySocket(path)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	outcome, _ := requestNativeRecoveryUntil(
		t.Context(), socket, 1, "initialize", map[string]any{}, time.Now().Add(120*time.Millisecond),
	)
	if outcome != nativeRecoveryAmbiguous || time.Since(started) > time.Second {
		t.Fatalf("deadline outcome=%v elapsed=%s", outcome, time.Since(started))
	}
	socket.close()
	_ = listener.Close()
	<-serverDone
}

func TestNativeRecoveryRejectsUnownedSocketPathForms(t *testing.T) {
	if _, err := connectNativeRecoverySocket("relative.sock"); err == nil {
		t.Fatal("relative recovery socket path was accepted")
	}
	root := t.TempDir()
	owned := nativeRecoveryTestSocket(t)
	listener, err := net.Listen("unix", owned)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	link := filepath.Join(root, "foreign.sock")
	if err := os.Symlink(owned, link); err != nil {
		t.Fatal(err)
	}
	if _, err := connectNativeRecoverySocket(link); err == nil {
		t.Fatal("symbolic-link recovery socket path was accepted")
	}
}

func runNativeRecoveryFixture(t *testing.T, scenario nativeRecoveryScenario) (NativeRecoveryOutcome, []map[string]any, bool) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	otherHome := filepath.Join(root, "other-home")
	otherWorkspace := filepath.Join(root, "other-workspace")
	for _, path := range []string{home, workspace, otherHome, otherWorkspace} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := nativeRecoveryTestSocket(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan []map[string]any, 1)
	go nativeRecoveryFixtureServer(listener, home, workspace, scenario, serverDone)
	socket, err := connectNativeRecoverySocket(path)
	if err != nil {
		t.Fatal(err)
	}
	var released atomic.Bool
	outcome := recoverNativeSessionOnSocket(
		t.Context(), socket, home, workspace, nativeRecoveryTestThread, scenario.expectedGoal,
		func() error { released.Store(true); return nil },
	)
	socket.close()
	calls := <-serverDone
	_ = listener.Close()
	return outcome, calls, released.Load()
}

func nativeRecoveryTestSocket(t *testing.T) string {
	t.Helper()
	// Test names and macOS's temporary root can exceed sockaddr_un's path limit.
	directory, err := os.MkdirTemp("/tmp", "gd-rpc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "rpc.sock")
}

func nativeRecoveryFixtureServer(
	listener net.Listener, home, workspace string, scenario nativeRecoveryScenario, done chan<- []map[string]any,
) {
	connection, err := listener.Accept()
	if err != nil {
		done <- nil
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	request, err := http.ReadRequest(reader)
	if err != nil {
		done <- nil
		return
	}
	_, _ = fmt.Fprintf(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", nativeWebSocketAccept(request.Header.Get("Sec-WebSocket-Key")))
	calls := []map[string]any{}
	for {
		value, err := readNativeTextFrame(reader)
		if err != nil {
			done <- calls
			return
		}
		var call map[string]any
		if json.Unmarshal([]byte(value), &call) != nil {
			done <- calls
			return
		}
		calls = append(calls, call)
		method, _ := call["method"].(string)
		if method == "initialized" {
			continue
		}
		result := map[string]any{}
		switch method {
		case "initialize":
			serverHome := home
			if scenario.wrongHome {
				serverHome = filepath.Dir(home) + "/other-home"
			}
			result["codexHome"] = serverHome
		case "thread/read":
			threadID, sessionID := nativeRecoveryTestThread, nativeRecoveryTestThread
			if scenario.wrongThread {
				threadID = "11111111-1111-4111-8111-111111111111"
			}
			if scenario.wrongSession {
				sessionID = "11111111-1111-4111-8111-111111111111"
			}
			cwd := workspace
			if scenario.wrongWorkspace {
				cwd = filepath.Dir(workspace) + "/other-workspace"
			}
			result["thread"] = map[string]any{
				"id": threadID, "sessionId": sessionID, "ephemeral": false,
				"canAcceptDirectInput": true, "cwd": cwd,
				"status": map[string]any{"type": scenario.status},
			}
		case "thread/goal/get":
			if scenario.goal == nil {
				result["goal"] = nil
			} else {
				used := 10
				if scenario.exhaustedBudget {
					used = 100
				}
				result["goal"] = map[string]any{
					"threadId": nativeRecoveryTestThread, "status": *scenario.goal,
					"tokenBudget": 100, "tokensUsed": used,
				}
			}
		case "thread/goal/set", "turn/start":
			if scenario.ambiguousMutation {
				returnDoneAndClose(done, calls, connection)
				return
			}
			if scenario.rejectMutation {
				writeNativeJSON(connection, call["id"], nil, true)
				continue
			}
			if method == "thread/goal/set" {
				result["goal"] = map[string]any{"threadId": nativeRecoveryTestThread, "status": "active"}
			} else {
				result["turn"] = map[string]any{"id": "native-continuation-turn", "status": "inProgress"}
			}
		default:
			returnDoneAndClose(done, calls, connection)
			return
		}
		writeNativeJSON(connection, call["id"], result, false)
	}
}

func returnDoneAndClose(done chan<- []map[string]any, calls []map[string]any, connection net.Conn) {
	done <- calls
	_ = connection.Close()
}

func readNativeTextFrame(reader *bufio.Reader) (string, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return "", err
	}
	payload, err := frame.ReadPayload(reader, nativeRecoveryMaxMessageSize)
	if err != nil {
		return "", err
	}
	if frame.Masked() {
		frame.Unmask(payload)
	}
	if frame.Opcode != 1 {
		return "", fmt.Errorf("unexpected fixture opcode %d", frame.Opcode)
	}
	return string(payload), nil
}

func writeNativeJSON(connection net.Conn, id any, result map[string]any, rejected bool) {
	response := map[string]any{"id": id}
	if rejected {
		response["error"] = map[string]any{"code": -32602, "message": "native policy rejected"}
	} else {
		response["result"] = result
	}
	encoded, _ := json.Marshal(response)
	_ = websocketframe.WriteFrame(connection, 1, encoded, false)
}

func nativeRecoveryMutations(calls []map[string]any) []map[string]any {
	mutations := make([]map[string]any, 0, 1)
	for _, call := range calls {
		method, _ := call["method"].(string)
		if method == "thread/goal/set" || method == "turn/start" {
			mutations = append(mutations, call)
		}
	}
	return mutations
}
