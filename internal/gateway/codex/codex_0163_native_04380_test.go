//go:build linux && amd64

package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProdex04380OfficialCodex0163LoopbackForegroundCompactionRetainsHistory(t *testing.T) {
	app, provider := newCodex0163AppServer(t, codex0163LoopbackComplete)
	initializeCodex0163AppServer(t, app)
	threadID := startCodex0163Thread(t, app)
	app.beginOperation()
	turn, err := app.request(3, "turn/start", map[string]any{"threadId": threadID, "input": []any{map[string]any{"type": "text", "text": "retain-this-user-message"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := turn.Result["turn"].(map[string]any); !ok {
		t.Fatalf("turn/start result missing turn: %#v", turn.Result)
	}
	if err := app.waitIdle(); err != nil {
		t.Fatal(err)
	}
	if provider.callCount() != 1 {
		t.Fatalf("completed turn upstream calls = %d, want 1", provider.callCount())
	}
	if _, err := provider.waitCall(app.ctx); err != nil {
		t.Fatal(err)
	}
	app.beginOperation()
	if _, err := app.request(4, "thread/compact/start", map[string]any{"threadId": threadID}); err != nil {
		t.Fatal(err)
	}
	if err := app.waitIdle(); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	compacted := app.compacted
	app.mu.Unlock()
	if !compacted {
		t.Fatal("native compaction did not emit a contextCompaction item")
	}
	if provider.callCount() != 2 {
		t.Fatalf("foreground compaction upstream calls = %d, want 2", provider.callCount())
	}
	call, err := provider.waitCall(app.ctx)
	if err != nil {
		t.Fatal(err)
	}
	provider.assertNoCall(t, 250*time.Millisecond)
	messages := codex0163InputMessages(call.body)
	userIndex, assistantIndex, checkpointIndex := -1, -1, -1
	for index, message := range messages {
		if message.Role == "user" && strings.Contains(message.Text, "retain-this-user-message") {
			userIndex = index
		}
		if message.Role == "assistant" && strings.Contains(message.Text, "synthetic answer") {
			assistantIndex = index
		}
		if message.Role == "user" && strings.Contains(message.Text, "CONTEXT CHECKPOINT COMPACTION") {
			checkpointIndex = index
		}
	}
	if userIndex < 0 || assistantIndex < 0 || checkpointIndex < 0 || !(userIndex < assistantIndex && assistantIndex < checkpointIndex) {
		t.Fatalf("compaction request lost or reordered retained history: %#v", messages)
	}
}

func TestProdex04380OfficialCodex0163LoopbackIncompleteUsageDoesNotReplay(t *testing.T) {
	app, provider := newCodex0163AppServer(t, codex0163LoopbackIncomplete)
	initializeCodex0163AppServer(t, app)
	threadID := startCodex0163Thread(t, app)
	app.beginOperation()
	if _, err := app.request(3, "turn/start", map[string]any{"threadId": threadID, "input": []any{map[string]any{"type": "text", "text": "incomplete fixture"}}}); err != nil {
		t.Fatal(err)
	}
	usageParams, completedParams, err := app.waitIncompleteTurn()
	if err != nil {
		t.Fatal(err)
	}
	if usageParams == nil {
		t.Fatal("turn completed without a native usage update")
	}
	total := usageParams["tokenUsage"].(map[string]any)["total"].(map[string]any)
	if total["inputTokens"] != float64(17) || total["outputTokens"] != float64(5) || total["totalTokens"] != float64(22) {
		t.Fatalf("incomplete usage = %#v, want input=17 output=5 total=22", total)
	}
	if completedParams["turn"].(map[string]any)["status"] != "failed" {
		t.Fatalf("incomplete turn terminal state = %#v", completedParams)
	}
	if !strings.Contains(completedParams["turn"].(map[string]any)["error"].(map[string]any)["message"].(string), "max_output_tokens") {
		t.Fatalf("incomplete reason was not retained: %#v", completedParams)
	}
	if provider.callCount() != 1 {
		t.Fatalf("incomplete response upstream calls = %d, want 1", provider.callCount())
	}
	if _, err := provider.waitCall(app.ctx); err != nil {
		t.Fatal(err)
	}
	provider.assertNoCall(t, 250*time.Millisecond)
}

func TestProdex04380OfficialCodex0163LoopbackInterruptCancelsWithoutReplay(t *testing.T) {
	app, provider := newCodex0163AppServer(t, codex0163LoopbackHold)
	initializeCodex0163AppServer(t, app)
	threadID := startCodex0163Thread(t, app)
	app.beginOperation()
	turn, err := app.request(3, "turn/start", map[string]any{"threadId": threadID, "input": []any{map[string]any{"type": "text", "text": "cancel fixture"}}})
	if err != nil {
		t.Fatal(err)
	}
	turnID := turn.Result["turn"].(map[string]any)["id"].(string)
	if _, err := provider.waitCall(app.ctx); err != nil {
		t.Fatal(err)
	}
	app.beginOperation()
	if _, err := app.request(4, "turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID}); err != nil {
		t.Fatal(err)
	}
	if err := app.waitIdle(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("turn/interrupt did not cancel the loopback response stream")
	}
	if provider.callCount() != 1 {
		t.Fatalf("cancelled turn upstream calls = %d, want 1", provider.callCount())
	}
	provider.assertNoCall(t, 250*time.Millisecond)
}

func initializeCodex0163AppServer(t *testing.T, app *codex0163AppServer) {
	t.Helper()
	if _, err := app.request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "godex-04380-proof", "version": "0"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.stdin.Write([]byte(`{"method":"initialized"}` + "\n")); err != nil {
		t.Fatal(err)
	}
}

func startCodex0163Thread(t *testing.T, app *codex0163AppServer) string {
	t.Helper()
	response, err := app.request(2, "thread/start", map[string]any{
		"cwd": app.command.Dir, "modelProvider": "loopback", "model": "fixture-model",
		"approvalPolicy": "never", "sandbox": "danger-full-access",
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := response.Result["thread"].(map[string]any)
	return thread["id"].(string)
}

type codex0163InputMessage struct {
	Role string
	Text string
}

func codex0163InputMessages(body []byte) []codex0163InputMessage {
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	items, _ := envelope["input"].([]any)
	messages := make([]codex0163InputMessage, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["type"] != "message" {
			continue
		}
		message := codex0163InputMessage{Role: fmt.Sprint(item["role"])}
		content, _ := item["content"].([]any)
		for _, rawPart := range content {
			part, _ := rawPart.(map[string]any)
			if text, ok := part["text"].(string); ok {
				message.Text += text
			}
		}
		messages = append(messages, message)
	}
	return messages
}
