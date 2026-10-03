//go:build !windows

package kiro

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestKiroLiveResponsesStreamEmitsBeforePromptCompletes(t *testing.T) {
	script := writeLiveKiroFixture(t, `
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":"first live"}}}}'
sleep 0.45
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":" second live"}}}}'
echo '{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}'
`)
	transport := newLiveKiroTransport(t, script)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/responses",
		Body: []byte(`{"model":"auto","input":"hello","stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	created := readUntilKiroStream(t, response.Body, reader, "resp_kiro_0", time.Second)
	if !strings.Contains(created, "response.created") {
		t.Fatalf("created event = %q", created)
	}
	first := readUntilKiroStream(t, response.Body, reader, "first live", 300*time.Millisecond)
	if !strings.Contains(first, "response.output_text.delta") {
		t.Fatalf("first live event = %q", first)
	}
	second := readUntilKiroStream(t, response.Body, reader, "second live", 2*time.Second)
	if !strings.Contains(second, "response.output_text.delta") {
		t.Fatalf("second live event = %q", second)
	}
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "response.output_item.done") ||
		!strings.Contains(string(rest), "response.completed") ||
		!strings.Contains(string(rest), "[DONE]") {
		t.Fatalf("final stream = %q", rest)
	}
}

func TestKiroLiveChatPreservesReasoningAndActivity(t *testing.T) {
	script := writeLiveKiroFixture(t, `
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"agent_thought_chunk","messageId":"thought","content":{"type":"text","text":"inspect code"}}}}'
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"tool_call","toolCallId":"tool1","title":"Read /home/test-user/private.txt","status":"in_progress","kind":"read","rawInput":{"path":"/home/test-user/private.txt"}}}}'
sleep 0.05
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"tool_call_update","toolCallId":"tool1","status":"completed","rawOutput":{"path":"/home/test-user/private.txt"}}}}'
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_live","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":"answer"}}}}'
sleep 0.35
echo '{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}'
`)
	transport := newLiveKiroTransport(t, script)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/chat/completions",
		Body: []byte(`{"model":"auto","messages":[{"role":"user","content":"hello"}],"stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	role := readUntilKiroStream(t, response.Body, reader, `"role":"assistant"`, time.Second)
	if !strings.Contains(role, "chat.completion.chunk") {
		t.Fatalf("role chunk = %q", role)
	}
	reasoning := readUntilKiroStream(t, response.Body, reader, `"reasoning_content":"inspect code"`, time.Second)
	if !strings.Contains(reasoning, "reasoning_content") {
		t.Fatalf("reasoning chunk = %q", reasoning)
	}
	activity := readUntilKiroStream(t, response.Body, reader, "phase=completed", time.Second)
	if strings.Index(activity, "phase=started") > strings.Index(activity, "phase=completed") ||
		strings.Contains(activity, "/home/test-user") ||
		strings.Contains(activity, "tool_calls") ||
		strings.Contains(activity, "function_call") {
		t.Fatalf("activity stream leaked/ordered incorrectly: %q", activity)
	}
	answer := readUntilKiroStream(t, response.Body, reader, `"content":"answer"`, time.Second)
	if !strings.Contains(answer, "answer") {
		t.Fatalf("answer chunk = %q", answer)
	}
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), `"finish_reason":"stop"`) || !strings.Contains(string(rest), "[DONE]") {
		t.Fatalf("chat final stream = %q", rest)
	}
}

func TestKiroLiveStreamCloseTerminatesProcessTree(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "parent.pid")
	childFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("KIRO_PARENT_PID_FILE", parentFile)
	t.Setenv("KIRO_CHILD_PID_FILE", childFile)
	t.Setenv("PRODEX_SUB_AGENT", "")
	script := writeLiveKiroFixture(t, `
echo $$ > "$KIRO_PARENT_PID_FILE"
sleep 30 &
echo $! > "$KIRO_CHILD_PID_FILE"
sleep 30
`)
	transport := newLiveKiroTransport(t, script)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/responses",
		Body: []byte(`{"model":"auto","input":"hello","stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	_ = readUntilKiroStream(t, response.Body, reader, "response.created", time.Second)
	parentPID := readPIDFile(t, parentFile)
	childPID := readPIDFile(t, childFile)
	start := time.Now()
	if err := response.Body.Close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitProcessGone(t, parentPID, 2*time.Second)
	waitProcessGone(t, childPID, 2*time.Second)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("stream close took too long to terminate tree: %s", elapsed)
	}
}

func newLiveKiroTransport(t *testing.T, script string) *RuntimeTransport {
	t.Helper()
	home := writeKiroRuntimeHome(t)
	source := NewSource()
	source.getenv = func(key string) string {
		if key == "PRODEX_KIRO_BIN" {
			return script
		}
		return os.Getenv(key)
	}
	transport, err := source.NewRuntimeTransport(context.Background(), home, "kiro-live")
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func writeLiveKiroFixture(t *testing.T, afterPrompt string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kiro-live.sh")
	content := `#!/bin/sh
set -eu
IFS= read -r init
IFS= read -r new_session
echo '{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1}}'
echo '{"jsonrpc":"2.0","id":1,"result":{"sessionId":"sess_live","models":{"currentModelId":"gpt-5.6-luna","availableModels":[{"modelId":"gpt-5.6-luna","name":"GPT-5.6 Luna"}]}}}'
IFS= read -r prompt
` + afterPrompt
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

type kiroReadResult struct {
	text string
	err  error
}

func readUntilKiroStream(
	t *testing.T,
	body io.Closer,
	reader *bufio.Reader,
	needle string,
	timeout time.Duration,
) string {
	t.Helper()
	result := make(chan kiroReadResult, 1)
	go func() {
		var output strings.Builder
		for {
			line, err := reader.ReadString('\n')
			output.WriteString(line)
			if strings.Contains(output.String(), needle) || err != nil {
				result <- kiroReadResult{text: output.String(), err: err}
				return
			}
		}
	}()
	select {
	case current := <-result:
		if current.err != nil && !errors.Is(current.err, io.EOF) {
			t.Fatalf("read stream until %q: %v; body=%q", needle, current.err, current.text)
		}
		if !strings.Contains(current.text, needle) {
			t.Fatalf("stream ended before %q: %q", needle, current.text)
		}
		return current.text
	case <-time.After(timeout):
		_ = body.Close()
		t.Fatalf("timed out waiting for %q", needle)
		return ""
	}
}

func TestReadPIDFileWaitsForShellWriteContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delayed.pid")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.WriteFile(path, []byte("4242\n"), 0o600)
	}()
	if got := readPIDFile(t, path); got != 4242 {
		t.Fatalf("PID = %d, want 4242", got)
	}
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var lastErr error
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			text := strings.TrimSpace(string(content))
			if text != "" {
				value, parseErr := strconv.Atoi(text)
				if parseErr == nil {
					return value
				}
				lastErr = parseErr
			} else {
				lastErr = errors.New("PID file is empty")
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Fatalf("PID file %s unavailable or incomplete: %v", path, lastErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still alive after %s (kill0=%v)", pid, timeout, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKiroLiveStreamIdleTimeoutCancelsSilentACP(t *testing.T) {
	t.Setenv("PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "120")
	script := writeLiveKiroFixture(t, `
sleep 30
`)
	transport := newLiveKiroTransport(t, script)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: "POST", Path: runtimeMountPath + "/responses",
		Body: []byte(`{"model":"auto","input":"hello","stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	_ = readUntilKiroStream(t, response.Body, reader, "response.created", time.Second)
	start := time.Now()
	_, err = io.ReadAll(reader)
	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("idle stream error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle timeout took too long: %s", elapsed)
	}
}
