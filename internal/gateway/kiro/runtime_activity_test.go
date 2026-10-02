package kiro

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKiroActivitySanitizesSensitiveAndPrivateLabels(t *testing.T) {
	private := kiroActivityItem("Read /home/test-user/private.txt", "failed", "read", false, true)
	if private["name"] != "read" || private["status"] != "failed" || private["phase"] != "failed" {
		t.Fatalf("private activity = %#v", private)
	}
	content, _ := json.Marshal(private)
	if strings.Contains(string(content), "/home/") {
		t.Fatalf("private path leaked: %s", content)
	}
	bearer := kiroActivityItem("Bearer secret-value", "RUNNING", "command", false, false)
	if bearer["name"] != "command" || bearer["status"] != "running" || bearer["phase"] != "updated" || bearer["kind"] != "command" {
		t.Fatalf("bearer activity = %#v", bearer)
	}
}

func TestKiroActivityBoundsUnicodeAndFormatsNonExecutableText(t *testing.T) {
	item := kiroActivityItem(strings.Repeat("界", 400), "malformed-status", "analysis", true, true)
	name := item["name"].(string)
	if name != strings.Repeat("界", 53) {
		t.Fatalf("bounded name bytes=%d runes=%d", len(name), len([]rune(name)))
	}
	if item["status"] != "unknown" || item["phase"] != "started" {
		t.Fatalf("activity = %#v", item)
	}
	text := kiroActivityText(item)
	if !strings.HasPrefix(text, "[Kiro activity: ") || !strings.Contains(text, "; details=omitted]") {
		t.Fatalf("activity text = %q", text)
	}
}

func TestKiroTurnActivityOmitsRawDetailsAndTruncatesCount(t *testing.T) {
	state := kiroTurnState{toolLabels: make(map[string]toolLabel)}
	state.applyToolActivity(true, map[string]any{
		"toolCallId": "one", "title": "Read file", "status": "in_progress", "kind": "read",
		"rawInput": map[string]any{"path": "/private/path"},
	})
	if len(state.toolActivities) != 1 || strings.Contains(state.assistantText, "/private/path") {
		t.Fatalf("state = %#v / %q", state.toolActivities, state.assistantText)
	}
	for index := 1; index < maxToolActivityEvents+4; index++ {
		state.applyToolActivity(false, map[string]any{
			"toolCallId": "id", "title": "activity", "status": "running",
		})
	}
	if len(state.toolActivities) != maxToolActivityEvents {
		t.Fatalf("activity count = %d", len(state.toolActivities))
	}
	last := state.toolActivities[len(state.toolActivities)-1].(map[string]any)
	if last["phase"] != "truncated" {
		t.Fatalf("last activity = %#v", last)
	}
}
