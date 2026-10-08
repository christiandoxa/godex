package superexpose

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProdex04356SuperExposeFullAdvertisesRunLifecycleWithoutLegacyNames(t *testing.T) {
	manager, err := newRunManager(t.TempDir(), nil, "gdxi_full", "repo")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.shutdown()
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.mode = "full"
	handler.runs = manager
	response := performMCP(t, handler, "tools/list", map[string]any{
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, "", true)
	if response.Code != 200 {
		t.Fatalf("tools/list status = %d: %s", response.Code, response.Body.String())
	}
	text := response.Body.String()
	for _, name := range []string{godexStartToolName, godexStatusToolName, godexEventsToolName, godexResultToolName, godexCancelToolName, godexListToolName, godexExecToolName} {
		if !strings.Contains(text, `"name":"`+name+`"`) {
			t.Fatalf("full tools missing %s: %s", name, text)
		}
	}
	for _, legacy := range []string{legacyStartToolName, legacyStatusToolName, legacyEventsToolName, legacyResultToolName, legacyCancelToolName, legacyListToolName, legacyExecToolName} {
		if strings.Contains(text, `"name":"`+legacy+`"`) {
			t.Fatalf("legacy tool leaked into advertisement: %s", legacy)
		}
	}
}

func TestProdex04356SuperExposeFullStartEventsResultAndLegacyIngress(t *testing.T) {
	t.Setenv(exposeHelperEnv, "run-success")
	manager, err := newRunManager(t.TempDir(), []string{"--no-sub-agent", "--no-presidio"}, "gdxi_full", "repo")
	if err != nil {
		t.Fatal(err)
	}
	manager.executable = os.Args[0]
	defer manager.shutdown()
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.mode = "full"
	handler.runs = manager

	secret := "sk-1234567890abcdef"
	started := performMCP(t, handler, "tools/call", map[string]any{
		"name":      legacyStartToolName,
		"arguments": map[string]any{"task": "review " + secret, "provider": "openai", "model": "gpt-test", "reasoning_effort": "high", "sub_agents": false},
		"_meta":     map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, legacyStartToolName, true)
	if started.Code != 200 {
		t.Fatalf("start = %d %s", started.Code, started.Body.String())
	}
	var rpc map[string]any
	if err := json.Unmarshal(started.Body.Bytes(), &rpc); err != nil {
		t.Fatal(err)
	}
	wrapper := rpc["result"].(map[string]any)
	structured := wrapper["structuredContent"].(map[string]any)
	runID, _ := structured["run_id"].(string)
	if !validRunID(runID) || !strings.HasPrefix(runID, "gsr_") {
		t.Fatalf("run id = %q", runID)
	}

	final := waitRunTerminal(t, manager, runID)
	if final["state"] != "succeeded" || final["provider"] != "openai" || final["model"] != "gpt-test" || final["reasoning_effort"] != "high" {
		t.Fatalf("final status = %#v", final)
	}
	result := manager.result(runID)
	if result["state"] == "unknown" {
		t.Fatal("result disappeared")
	}
	output, _ := result["output"].(string)
	if strings.Contains(output, secret) || !strings.Contains(output, "<redacted>") || !strings.Contains(output, "run-stderr") {
		t.Fatalf("bounded/redacted result = %q", output)
	}
	events := manager.events(runID, 0, maxEventPage)
	if events["state"] == "unknown" {
		t.Fatal("events disappeared")
	}
	if events["run_id"] != runID {
		t.Fatalf("events envelope = %#v", events)
	}
	page := events["events"].([]map[string]any)
	if len(page) == 0 {
		t.Fatalf("events page empty: %#v", events)
	}
	list := manager.list()
	if len(list) != 1 || list[0]["run_id"] != runID {
		t.Fatalf("run list = %#v", list)
	}
}

func TestProdex04356SuperExposeFullRunLifecycleIsAudited(t *testing.T) {
	t.Setenv(exposeHelperEnv, "run-success")
	manager, err := newRunManager(t.TempDir(), nil, "gdxi_audit", "repo")
	if err != nil {
		t.Fatal(err)
	}
	manager.executable = os.Args[0]
	defer manager.shutdown()
	capture := &exposeAuditCapture{}
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.mode = "full"
	handler.runs = manager
	handler.audit = newExposeAuditLogWithSink(t.Context(), capture)

	started := performMCP(t, handler, "tools/call", map[string]any{
		"name": godexStartToolName,
		"arguments": map[string]any{
			"task": "audit-run",
		},
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, godexStartToolName, true)
	if started.Code != 200 {
		t.Fatalf("start = %d %s", started.Code, started.Body.String())
	}
	var rpc map[string]any
	if err := json.Unmarshal(started.Body.Bytes(), &rpc); err != nil {
		t.Fatal(err)
	}
	structured := rpc["result"].(map[string]any)["structuredContent"].(map[string]any)
	runID := structured["run_id"].(string)
	waitRunTerminal(t, manager, runID)

	for _, kind := range []string{
		"super_expose_run_created",
		"super_expose_run_started",
		"super_expose_run_completed",
	} {
		event := capture.waitEvent(kind)
		if event == nil {
			t.Fatalf("missing %s", kind)
		}
		if event.Fields["run_id"] != runID {
			t.Fatalf("%s run_id = %#v", kind, event.Fields)
		}
	}
	completed := capture.event("super_expose_run_completed")
	if completed.Fields["state"] != "succeeded" || completed.Fields["exit_code"] != "0" {
		t.Fatalf("run completion audit = %#v", completed.Fields)
	}
}

func TestProdex04356SuperExposeFullQueueBoundsAndCancellation(t *testing.T) {
	t.Setenv(exposeHelperEnv, "run-sleep")
	manager, err := newRunManager(t.TempDir(), nil, "gdxi_queue", "repo")
	if err != nil {
		t.Fatal(err)
	}
	manager.executable = os.Args[0]
	defer manager.shutdown()
	ids := make([]string, 0, maxActiveRuns+maxQueuedRuns)
	for index := 0; index < maxActiveRuns+maxQueuedRuns; index++ {
		started, err := manager.start(map[string]any{"task": "sleep"})
		if err != nil {
			t.Fatalf("start %d: %v", index, err)
		}
		ids = append(ids, started["run_id"].(string))
	}
	if _, err := manager.start(map[string]any{"task": "overflow"}); err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("overflow start = %v", err)
	}
	queuedID := ids[len(ids)-1]
	queued := manager.cancel(queuedID)
	if queued["state"] != "cancelled" || queued["cancellation_requested"] != true {
		t.Fatalf("queued cancel = %#v", queued)
	}
	runningID := ids[0]
	running := manager.cancel(runningID)
	if running["cancellation_requested"] != true {
		t.Fatalf("running cancel = %#v", running)
	}
	final := waitRunState(t, manager, runningID, "cancelled")
	if final["state"] != "cancelled" {
		t.Fatalf("running final = %#v", final)
	}
}

func TestProdex04356SuperExposeFullOutputBoundAndFailureState(t *testing.T) {
	manager, err := newRunManager(t.TempDir(), nil, "gdxi_output", "repo")
	if err != nil {
		t.Fatal(err)
	}
	manager.executable = os.Args[0]
	defer manager.shutdown()
	t.Setenv(exposeHelperEnv, "run-large")
	started, err := manager.start(map[string]any{"task": "large"})
	if err != nil {
		t.Fatal(err)
	}
	largeID := started["run_id"].(string)
	waitRunTerminal(t, manager, largeID)
	large := manager.result(largeID)
	output := large["output"].(string)
	if len(output) != runOutputMaxBytes || large["output_truncated"] != true {
		t.Fatalf("large result len=%d truncated=%v", len(output), large["output_truncated"])
	}

	t.Setenv(exposeHelperEnv, "run-fail")
	failed, err := manager.start(map[string]any{"task": "fail"})
	if err != nil {
		t.Fatal(err)
	}
	failedID := failed["run_id"].(string)
	status := waitRunTerminal(t, manager, failedID)
	if status["state"] != "failed" || status["exit_status"] != 23 {
		t.Fatalf("failed status = %#v", status)
	}
}

func TestProdex04356SuperExposeFullToolArgumentValidation(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{godexStartToolName, map[string]any{"task": "x", "unexpected": true}},
		{godexStatusToolName, map[string]any{"run_id": "gsr_fixture", "limit": 1}},
		{godexListToolName, map[string]any{"unexpected": true}},
	}
	for _, tc := range cases {
		if err := validateToolArguments(tc.name, tc.args); err == nil || !strings.Contains(err.Error(), "unknown tool argument") {
			t.Fatalf("%s validation = %v", tc.name, err)
		}
	}
	if _, err := requiredRunID(map[string]any{"run_id": "bad/id"}); err == nil {
		t.Fatal("invalid run id accepted")
	}
	if _, err := optionalUint(map[string]any{"limit": json.Number("-1")}, "limit", 64); err == nil {
		t.Fatal("negative integer accepted")
	}
}

func waitRunTerminal(t *testing.T, manager *runManager, runID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		value := manager.status(runID)
		switch value["state"] {
		case "succeeded", "failed", "cancelled", "start_failed":
			return value
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not become terminal: %#v", runID, manager.status(runID))
	return nil
}

func waitRunState(t *testing.T, manager *runManager, runID, state string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		value := manager.status(runID)
		if value["state"] == state {
			return value
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach %s: %#v", runID, state, manager.status(runID))
	return nil
}
