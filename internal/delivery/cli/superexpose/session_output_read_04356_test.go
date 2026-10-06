package superexpose

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProdex04356OutputReadHidesInternalContextAndKeepsVisibleUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	hidden := `{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hidden instructions"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["agents_md.instructions"]}}}`
	turn := `{"timestamp":"2026-09-03T10:00:01Z","type":"turn_context","payload":{"model":"hidden-model","cwd":"/hidden"}}`
	visible := `{"timestamp":"2026-09-03T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"visible prompt"}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["user.text"]}}}`
	if err := os.WriteFile(path, []byte(hidden+"\n"+turn+"\n"+visible+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 1 || batch.events[0].Kind != "user" || batch.events[0].Text != "visible prompt" {
		t.Fatalf("filtered events = %#v", batch.events)
	}
}

func TestProdex04356OutputReadRecoversMalformedUTF8AndOversizedWithoutPayloadLeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	oversized := `{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"function_call_output","output":"` + strings.Repeat("x", 200_000) + `"}}`
	assistant := `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"recovered"}}`
	content := append([]byte("not-json\n"), 0xff, '\n')
	content = append(content, []byte(oversized+"\n"+assistant+"\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 4 {
		t.Fatalf("recovery events = %#v", batch.events)
	}
	wantNames := []any{"malformed_record", "invalid_utf8", "oversized_record", nil}
	for index, event := range batch.events {
		if event.Name != wantNames[index] {
			t.Fatalf("event %d name = %#v want %#v", index, event.Name, wantNames[index])
		}
		if strings.Contains(event.Text, "not-json") || strings.Contains(event.Text, strings.Repeat("x", 32)) {
			t.Fatalf("gap leaked raw payload: %#v", event)
		}
	}
	if batch.events[3].Kind != "assistant" || batch.events[3].Text != "recovered" {
		t.Fatalf("recovered event = %#v", batch.events[3])
	}
}

func TestProdex04356OutputReadNearLimitUserIsBoundedNotDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	message := strings.Repeat("\\", 64*1024-1024)
	record := map[string]any{
		"timestamp": "2026-09-03T10:00:00Z",
		"type":      "response_item",
		"payload": map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{"type": "input_text", "text": message}},
		},
	}
	encoded, _ := json.Marshal(record)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 1 || batch.events[0].Kind != "user" ||
		len(batch.events[0].Text) > outputReadMaxTextBytes ||
		!strings.Contains(batch.events[0].Text, "[text_truncated]") {
		t.Fatalf("near-limit user event = %#v", batch.events)
	}
}

func TestProdex04356OutputReadIncompleteFinalAppendDoesNotAdvance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	record := `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"partial"}}`
	if err := os.WriteFile(path, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 0 || batch.nextOffset != 0 || !batch.hasMore {
		t.Fatalf("partial batch = %#v", batch)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	batch, err = readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 1 || batch.events[0].Text != "partial" {
		t.Fatalf("completed batch = %#v", batch)
	}
}

func TestProdex04356OutputCursorAcceptsGodexAndLegacyProdexPIDFields(t *testing.T) {
	cursor := outputCursor{
		version:  outputCursorVersion,
		godexPID: 100, godexBirth: "birth-godex",
		codexPID: 101, codexBirth: "birth-codex",
		threadID: "00000000-0000-4000-8000-000000000123",
		sourceID: "source", offset: 44, eventIndex: 2, checkpointID: "checkpoint",
	}
	canonical, err := encodeOutputCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeOutputCursor(canonical)
	if err != nil || decoded != cursor {
		t.Fatalf("canonical cursor = %#v err=%v", decoded, err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(canonical)
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value["prodex_pid"] = value["godex_pid"]
	value["prodex_birth"] = value["godex_birth"]
	delete(value, "godex_pid")
	delete(value, "godex_birth")
	raw, _ = json.Marshal(value)
	legacy := base64.RawURLEncoding.EncodeToString(raw)
	decoded, err = decodeOutputCursor(legacy)
	if err != nil || decoded.godexPID != 100 || decoded.godexBirth != "birth-godex" {
		t.Fatalf("legacy cursor = %#v err=%v", decoded, err)
	}
}

type outputSessionQueue struct {
	path string
}

func (queue outputSessionQueue) checkCapability(resolvedSessionTarget) error  { return nil }
func (queue outputSessionQueue) persistedThread(string, string) (bool, error) { return true, nil }
func (queue outputSessionQueue) rolloutPath(string, string) (string, error)   { return queue.path, nil }
func (queue outputSessionQueue) queueOnce(resolvedSessionTarget, string) queueInvocation {
	code := 0
	return queueInvocation{outcome: queueAccepted, exitCode: &code, queued: true}
}
func (queue outputSessionQueue) preempt(resolvedSessionTarget) (queuePreemptResult, error) {
	return queuePreemptResult{}, sessionQueueUnsupported
}
func (queue outputSessionQueue) loadedThreadAddressable(resolvedSessionTarget) (bool, error) {
	return true, nil
}

func TestProdex04356SessionOutputCursorContinuesAppendAndRejectsRewriteBeforeCursor(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	sessionDir := filepath.Join(fixture.codexHome, "sessions", "2026", "09", "03")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(sessionDir, "rollout-2026-09-03T10-00-00-"+fixture.threadID+".jsonl")
	firstLine := `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"first"}}` + "\n"
	if err := os.WriteFile(rollout, []byte(firstLine), 0o600); err != nil {
		t.Fatal(err)
	}
	service := newExistingSessionService(fixture.process, outputSessionQueue{path: rollout})
	first, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, limit: 10, bindingKey: "cursor",
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first["next_cursor"].(string)
	events := first["events"].([]map[string]any)
	if len(events) != 1 || events[0]["text"] != "first" {
		t.Fatalf("first read = %#v", first)
	}

	file, err := os.OpenFile(rollout, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondLine := `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"second"}}` + "\n"
	if _, err := file.WriteString(secondLine); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	second, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, cursor: cursor, limit: 10, bindingKey: "cursor",
	})
	if err != nil {
		t.Fatal(err)
	}
	events = second["events"].([]map[string]any)
	if len(events) != 1 || events[0]["text"] != "second" {
		t.Fatalf("append continuation = %#v", second)
	}

	rewrite, err := os.OpenFile(rollout, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rewrite.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	_ = rewrite.Close()
	_, err = service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, cursor: cursor, limit: 10, bindingKey: "cursor",
	})
	if !errors.Is(err, sessionOutputSourceChanged) {
		t.Fatalf("rewrite before cursor = %v", err)
	}
}

func TestProdex04356SessionOutputWaitTimeoutReturnsEmptySuccess(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	sessionDir := filepath.Join(fixture.codexHome, "sessions", "2026", "09", "03")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(sessionDir, "rollout-2026-09-03T10-00-00-"+fixture.threadID+".jsonl")
	line := `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"first"}}` + "\n"
	if err := os.WriteFile(rollout, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	service := newExistingSessionService(fixture.process, outputSessionQueue{path: rollout})
	first, err := service.readOutput(outputReadRequest{workspaceRoot: fixture.workspace, limit: 10, bindingKey: "wait"})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	next, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, cursor: first["next_cursor"].(string),
		limit: 10, waitMS: 120, bindingKey: "wait",
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("wait elapsed = %v", elapsed)
	}
	if events := next["events"].([]map[string]any); len(events) != 0 {
		t.Fatalf("wait events = %#v", events)
	}
}
