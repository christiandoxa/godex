package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionMetadataRepairPlanPreservesValidSubagentMetadata(t *testing.T) {
	sessionID := "01900000-0000-7000-8000-000000000001"
	path := filepath.Join(t.TempDir(), "rollout-2026-07-11T11-17-19-"+sessionID+".jsonl")
	raw := "{\"timestamp\":\"2026-07-11T11:17:19Z\",\"type\":\"session_meta\",\"payload\":{" +
		"\"session_id\":\"01900000-0000-7000-8000-000000000002\"," +
		"\"id\":\"" + sessionID + "\",\"timestamp\":\"2026-07-11T11:17:19Z\"," +
		"\"cwd\":\"/tmp/workspace\",\"originator\":\"codex-tui\",\"cli_version\":\"0.144.1\"," +
		"\"source\":{\"subagent\":{\"thread_spawn\":{\"parent_thread_id\":\"parent\",\"depth\":1}}}}}\n"
	repaired, changed, err := planSessionMetadataRepair(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	if changed || repaired != raw {
		t.Fatalf("valid metadata changed = %t, %q", changed, repaired)
	}
}

func TestSessionMetadataRepairPlanMovesMatchingCodexMetadataAndDropsDuplicates(t *testing.T) {
	sessionID := "019ebd01-c881-74c0-b01d-7fdf5bd4dd32"
	path := filepath.Join(t.TempDir(), "rollout-2026-06-13T02-04-31-"+sessionID+".jsonl")
	meta := "{\"timestamp\":\"2026-06-13T02:04:32Z\",\"type\":\"session_meta\",\"payload\":{" +
		"\"id\":\"" + sessionID + "\",\"timestamp\":\"2026-06-13T02:04:32Z\",\"cwd\":\"/tmp/workspace\"," +
		"\"originator\":\"codex-tui\",\"cli_version\":\"0.160.0\"}}"
	event1 := "{\"timestamp\":\"2026-06-13T02:04:31Z\",\"type\":\"event\",\"payload\":{\"message\":\"before\"}}"
	event2 := "{\"timestamp\":\"2026-06-13T02:04:33Z\",\"type\":\"event\",\"payload\":{\"message\":\"after\"}}"
	raw := event1 + "\n" + meta + "\n" + meta + "\n" + event2 + "\n"
	repaired, changed, err := planSessionMetadataRepair(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("late metadata was not repaired")
	}
	lines := strings.Split(strings.TrimSpace(repaired), "\n")
	if len(lines) != 3 || lines[0] != meta || lines[1] != event1 || lines[2] != event2 {
		t.Fatalf("repaired lines = %#v", lines)
	}
}

func TestSessionMetadataRepairPlanSynthesizesTaggedMetadata(t *testing.T) {
	sessionID := "019ec6c3-28a4-79f0-91f9-74a2f34b0928"
	path := filepath.Join(t.TempDir(), "rollout-2026-06-14T23-32-19-"+sessionID+".jsonl")
	raw := "{\"timestamp\":\"2026-06-14T23:32:19Z\",\"type\":\"event\",\"payload\":{" +
		"\"message\":\"partial only\",\"cwd\":\"/tmp/workspace\",\"model_provider\":\"prodex-deepseek\"}}\n"
	repaired, changed, err := planSessionMetadataRepair(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("missing metadata was not synthesized")
	}
	lines := strings.Split(strings.TrimSpace(repaired), "\n")
	if len(lines) != 2 || lines[1] != strings.TrimSpace(raw) {
		t.Fatalf("repaired lines = %#v", lines)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		t.Fatal(err)
	}
	payload := meta["payload"].(map[string]any)
	if meta["timestamp"] != "2026-06-14T23:32:19Z" || meta["type"] != "session_meta" ||
		payload["id"] != sessionID || payload["session_id"] != sessionID ||
		payload["timestamp"] != "2026-06-14T23:32:19Z" || payload["cwd"] != "/tmp/workspace" ||
		payload["originator"] != "prodex-repair" || payload["cli_version"] != "0.435.1" ||
		payload["source"] != "cli" || payload["model_provider"] != "prodex-deepseek" {
		t.Fatalf("synthetic metadata = %#v", meta)
	}
	if !sessionLineStartsCodexRolloutMetadata(lines[0]) {
		t.Fatalf("synthetic metadata is not Codex rollout metadata: %s", lines[0])
	}
}

func TestSessionMetadataRepairPlanDropsCorruptLinesAndUpgradesMinimalMetadata(t *testing.T) {
	sessionID := "019ec6c3-28a4-79f0-91f9-74a2f34b0928"
	path := filepath.Join(t.TempDir(), "rollout-"+sessionID+".jsonl")
	minimal := "{\"payload\":{\"id\":\"" + sessionID + "\"},\"type\":\"session_meta\"}"
	event1 := "{\"timestamp\":\"2026-06-14T23:32:19Z\",\"type\":\"event\",\"payload\":{\"message\":\"first readable chat\"}}"
	event2 := "{\"timestamp\":\"2026-06-14T23:32:20Z\",\"type\":\"event\",\"payload\":{\"message\":\"second readable chat\"}}"
	raw := minimal + "\n" + event1 + "\n{not-json\n\n" + event2 + "\n"
	repaired, changed, err := planSessionMetadataRepair(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("minimal/corrupt session was not repaired")
	}
	lines := strings.Split(strings.TrimSpace(repaired), "\n")
	if len(lines) != 3 || !sessionLineStartsCodexRolloutMetadata(lines[0]) ||
		lines[1] != event1 || lines[2] != event2 || strings.Contains(repaired, "{not-json") {
		t.Fatalf("repaired session = %#v", lines)
	}
}

func TestSessionIDFromPathMatchesTaggedUUIDExtraction(t *testing.T) {
	id := "019ec6c3-28a4-79f0-91f9-74a2f34b0928"
	for _, name := range []string{id + ".jsonl", "rollout-2026-06-14T23-32-19-" + id + ".jsonl"} {
		got, ok := sessionIDFromPath(filepath.Join(t.TempDir(), name))
		if !ok || got != id {
			t.Fatalf("session ID from %q = %q,%t", name, got, ok)
		}
	}
	for _, name := range []string{
		"rollout-no-id.jsonl",
		"rollout-019ec6c3-28a4-79f0-91f9.jsonl",
		"rollout-" + id + ".jsonl.zst",
	} {
		if got, ok := sessionIDFromPath(filepath.Join(t.TempDir(), name)); ok {
			t.Fatalf("invalid session path %q produced %q", name, got)
		}
	}
}
