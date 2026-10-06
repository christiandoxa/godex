package superexpose

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type fakeSessionProcessInspector struct {
	uid     uint32
	listed  []processRecord
	details map[uint32]*processDetails
}

func (fake fakeSessionProcessInspector) currentUID() (uint32, error) { return fake.uid, nil }
func (fake fakeSessionProcessInspector) list() ([]processRecord, error) {
	return append([]processRecord(nil), fake.listed...), nil
}
func (fake fakeSessionProcessInspector) inspect(pid uint32) (*processDetails, error) {
	value := fake.details[pid]
	if value == nil {
		return nil, nil
	}
	copy := *value
	copy.openFiles = append([]openProcessFile(nil), value.openFiles...)
	return &copy, nil
}

type fakeSessionQueue struct {
	capabilityErr error
	rollout       string
	invocation    queueInvocation
	preempted     queuePreemptResult
	preemptErr    error
}

func (fake fakeSessionQueue) checkCapability(resolvedSessionTarget) error  { return fake.capabilityErr }
func (fake fakeSessionQueue) persistedThread(string, string) (bool, error) { return true, nil }
func (fake fakeSessionQueue) rolloutPath(string, string) (string, error)   { return fake.rollout, nil }
func (fake fakeSessionQueue) queueOnce(resolvedSessionTarget, string) queueInvocation {
	if fake.invocation.outcome == 0 && !fake.invocation.queued && fake.invocation.messageID == "" {
		return queueInvocation{outcome: queueAccepted, queued: true}
	}
	return fake.invocation
}
func (fake fakeSessionQueue) preempt(resolvedSessionTarget) (queuePreemptResult, error) {
	if fake.preemptErr != nil {
		return queuePreemptResult{}, fake.preemptErr
	}
	return fake.preempted, nil
}
func (fake fakeSessionQueue) loadedThreadAddressable(resolvedSessionTarget) (bool, error) {
	return true, nil
}

func TestProdex04356SessionResolverSelectsOnePlainGodexAndDescendantWriter(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})

	target, err := service.resolveSessionTarget(fixture.workspace, nil, fixture.threadID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if target.godex.pid != 100 || target.writer.pid != 101 || target.threadID != fixture.threadID {
		t.Fatalf("resolved target = %#v", target)
	}
	if target.queueDB != fixture.queueDB || target.stateDB != fixture.stateDB {
		t.Fatalf("database target = queue:%q state:%q", target.queueDB, target.stateDB)
	}
	if target.environment.codexHome != fixture.codexHome || target.environment.pwd != fixture.workspace {
		t.Fatalf("target environment = %#v", target.environment)
	}
}

func TestProdex04356SessionResolverRejectsAmbiguityAndMissingBirthIdentity(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	other := fixture.process.listed[0]
	other.pid = 200
	other.birthIdentity = "birth-godex-2"
	fixture.process.listed = append(fixture.process.listed, other)
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})

	if _, err := service.resolveTarget(fixture.workspace, nil); !errors.Is(err, sessionAmbiguousSession) {
		t.Fatalf("ambiguous Godex sessions = %v", err)
	}

	fixture = newSessionResolverFixture(t)
	fixture.process.listed[0].birthIdentity = ""
	service = newExistingSessionService(fixture.process, fakeSessionQueue{})
	if _, err := service.resolveTarget(fixture.workspace, nil); !errors.Is(err, sessionVerificationInconclusive) {
		t.Fatalf("missing Godex birth identity = %v", err)
	}

	fixture = newSessionResolverFixture(t)
	writer2 := fixture.process.listed[1]
	writer2.pid = 102
	writer2.birthIdentity = "birth-codex-2"
	fixture.process.listed = append(fixture.process.listed, writer2)
	fixture.process.details[102] = fixture.process.details[101]
	service = newExistingSessionService(fixture.process, fakeSessionQueue{})
	if _, err := service.resolveWriter(fixture.process.listed[0], fixture.workspace); !errors.Is(err, sessionAmbiguousCodexWriter) {
		t.Fatalf("ambiguous writers = %v", err)
	}
}

func TestProdex04356SessionResolverFailsClosedOnThreadIdentityConflict(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	other := "11111111-1111-4111-8111-111111111111"
	fixture.process.details[101].openFiles = append(
		fixture.process.details[101].openFiles,
		openProcessFile{path: filepath.Join(fixture.codexHome, "thread-writer-locks", other+".lock")},
	)
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})
	if _, err := service.resolveWriter(fixture.process.listed[0], fixture.workspace); !errors.Is(err, sessionThreadIdentityConflict) {
		t.Fatalf("thread conflict = %v", err)
	}
}

func TestProdex04356SessionResolverExplicitMissingTargetBecomesStale(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	fixture.process.listed = nil
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})
	pid := uint32(100)
	_, err := service.resolveSessionTarget(fixture.workspace, &pid, fixture.threadID, nil)
	if !errors.Is(err, sessionStaleTarget) {
		t.Fatalf("explicit missing target = %v", err)
	}
}

func TestProdex04356SessionResolverBindingRejectsBirthIdentityChange(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})
	target, err := service.resolveSessionTarget(fixture.workspace, nil, fixture.threadID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.rememberBinding("binding", target, ""); err != nil {
		t.Fatal(err)
	}
	binding, err := service.binding("binding")
	if err != nil {
		t.Fatal(err)
	}
	changed := target
	changed.writer.birthIdentity = "different"
	if err := service.verifyBinding(binding, changed); !errors.Is(err, sessionStaleTarget) {
		t.Fatalf("birth identity mutation = %v", err)
	}
}

func TestProdex04356SessionResolverFiltersWrongUIDWorkspaceAndRemoteWriter(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	wrongUID := fixture.process.listed[0]
	wrongUID.pid = 300
	wrongUID.uid = 2000
	wrongUID.birthIdentity = "other"
	wrongWorkspace := fixture.process.listed[0]
	wrongWorkspace.pid = 301
	wrongWorkspace.cwd = t.TempDir()
	wrongWorkspace.birthIdentity = "other2"
	remoteWriter := fixture.process.listed[1]
	remoteWriter.pid = 302
	remoteWriter.argv = []string{"codex", "--remote", "ws://example.test", "resume"}
	remoteWriter.birthIdentity = "remote"
	fixture.process.listed = append(fixture.process.listed, wrongUID, wrongWorkspace, remoteWriter)
	service := newExistingSessionService(fixture.process, fakeSessionQueue{})

	target, err := service.resolveSessionTarget(fixture.workspace, nil, fixture.threadID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if target.writer.pid != 101 {
		t.Fatalf("filtered writer = %#v", target.writer)
	}
}

type sessionResolverFixture struct {
	workspace string
	codexHome string
	threadID  string
	queueDB   string
	stateDB   string
	process   fakeSessionProcessInspector
}

func newSessionResolverFixture(t *testing.T) sessionResolverFixture {
	t.Helper()
	workspace := t.TempDir()
	codexHome := t.TempDir()
	threadID := "00000000-0000-4000-8000-000000000123"
	lockDir := filepath.Join(codexHome, "thread-writer-locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(lockDir, threadID+".lock")
	queueDB := filepath.Join(codexHome, "queue_1.sqlite")
	stateDB := filepath.Join(codexHome, "state_5.sqlite")
	for _, path := range []string{lock, queueDB, stateDB} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	godex := processRecord{
		pid: 100, parentPID: 1, uid: 1000, state: processRunning,
		executable: "/usr/bin/godex", argv: []string{"godex", "s"},
		cwd: workspace, startTime: "100", birthIdentity: "birth-godex",
	}
	writer := processRecord{
		pid: 101, parentPID: 100, uid: 1000, state: processRunning,
		executable: "/usr/bin/codex", argv: []string{"codex", "resume", threadID},
		cwd: workspace, startTime: "101", birthIdentity: "birth-codex",
	}
	details := &processDetails{
		record: writer,
		environment: targetEnvironment{
			home: workspace, codexHome: codexHome, codexSQLiteHome: codexHome, pwd: workspace,
		},
		openFiles: []openProcessFile{
			{path: lock}, {path: queueDB}, {path: stateDB},
		},
	}
	return sessionResolverFixture{
		workspace: workspace, codexHome: codexHome, threadID: threadID,
		queueDB: queueDB, stateDB: stateDB,
		process: fakeSessionProcessInspector{
			uid: 1000, listed: []processRecord{godex, writer},
			details: map[uint32]*processDetails{101: details},
		},
	}
}

func TestProdex04356SessionWritePreemptAndOutputReadUseOneBoundIdentity(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	rolloutDir := filepath.Join(fixture.codexHome, "sessions")
	if err := os.MkdirAll(rolloutDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(rolloutDir, "rollout-"+fixture.threadID+".jsonl")
	initial := strings.Join([]string{
		"{\"timestamp\":\"t1\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"hello\"}}",
		"{\"timestamp\":\"t2\",\"type\":\"response_item\",\"payload\":{\"type\":\"reasoning\",\"summary\":[\"hidden\"]}}",
		"{\"timestamp\":\"t3\",\"type\":\"response_item\",\"payload\":{\"type\":\"function_call\",\"name\":\"shell\",\"arguments\":\"pwd\"}}",
		"not-json",
	}, "\n") + "\n"
	if err := os.WriteFile(rollout, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := fakeSessionQueue{
		rollout: rollout,
		invocation: queueInvocation{
			outcome: queueAccepted, queued: true,
			messageID:    "11111111-1111-4111-8111-111111111111",
			submissionID: "submission-one",
		},
		preempted: queuePreemptResult{
			currentTurnID: "turn-one", currentTurnInterrupted: true,
			cancelledSubmissionIDs: []string{"queued-one"},
			remainingSubmissionIDs: nil,
			queueEmptyAtBoundary:   true, sessionReady: true,
		},
	}
	service := newExistingSessionService(fixture.process, queue)
	binding := sessionBindingKey("gdxi-test", nil, fixture.threadID)

	written, err := service.write(sessionPromptWriteRequest{
		workspaceRoot: fixture.workspace, message: "next task",
		threadID: fixture.threadID, bindingKey: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	if written["status"] != "written" || written["message_id"] != "11111111-1111-4111-8111-111111111111" ||
		written["submission_id"] != "submission-one" || written["verification"] != "queue_pending_observed" {
		t.Fatalf("prompt write = %#v", written)
	}
	cursor, _ := written["output_cursor"].(string)
	if cursor == "" {
		t.Fatalf("prompt write cursor = %#v", written)
	}
	decoded, err := decodeOutputCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.threadID != fixture.threadID || decoded.godexPID != 100 || decoded.codexPID != 101 {
		t.Fatalf("prompt cursor = %#v", decoded)
	}

	preempted, err := service.preempt(sessionPreemptRequest{
		workspaceRoot: fixture.workspace, threadID: fixture.threadID, bindingKey: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preempted["status"] != "preempted" || preempted["current_turn_id"] != "turn-one" ||
		preempted["cancelled_count"] != 1 || preempted["remaining_count"] != 0 ||
		preempted["session_ready"] != true {
		t.Fatalf("preempt = %#v", preempted)
	}

	read, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, threadID: fixture.threadID,
		bindingKey: binding, limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	events, ok := read["events"].([]map[string]any)
	if !ok {
		t.Fatalf("output events type = %T %#v", read["events"], read["events"])
	}
	if len(events) != 3 {
		t.Fatalf("output events = %#v", events)
	}
	if events[0]["kind"] != "assistant" || events[0]["text"] != "hello" ||
		events[1]["kind"] != "tool" || events[1]["status"] != "started" ||
		events[2]["kind"] != "gap" || events[2]["name"] != "malformed_record" {
		t.Fatalf("output event mapping = %#v", events)
	}
	next, _ := read["next_cursor"].(string)
	if next == "" {
		t.Fatalf("next cursor = %#v", read)
	}

	file, err := os.OpenFile(rollout, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("{\"timestamp\":\"t4\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"later\"}}\n")
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	follow, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, cursor: next, limit: 100,
		bindingKey: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	followEvents := follow["events"].([]map[string]any)
	if len(followEvents) != 1 || followEvents[0]["kind"] != "assistant" || followEvents[0]["text"] != "later" {
		t.Fatalf("cursor continuation = %#v", follow)
	}
}

func TestProdex04356SessionOutputCursorFailsClosedAfterPrefixMutation(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	rolloutDir := filepath.Join(fixture.codexHome, "sessions")
	if err := os.MkdirAll(rolloutDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(rolloutDir, "rollout-"+fixture.threadID+".jsonl")
	line := "{\"timestamp\":\"t1\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"original\"}}\n"
	if err := os.WriteFile(rollout, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	service := newExistingSessionService(fixture.process, fakeSessionQueue{rollout: rollout})
	read, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, threadID: fixture.threadID,
		bindingKey: "binding", limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor := read["next_cursor"].(string)
	mutated := strings.Replace(line, "original", "mutated!", 1)
	if len(mutated) != len(line) {
		t.Fatal("test mutation changed file length")
	}
	if err := os.WriteFile(rollout, []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.readOutput(outputReadRequest{
		workspaceRoot: fixture.workspace, cursor: cursor, bindingKey: "binding",
	}); !errors.Is(err, sessionOutputSourceChanged) {
		t.Fatalf("mutated cursor source = %v", err)
	}
}

func TestProdex04356SessionToolAdvertisementIsFullModeOnlyAndGodexNative(t *testing.T) {
	full := &execMCPHandler{mode: "full", displayName: "repo", instanceID: "gdxi-test"}
	encoded, err := json.Marshal(full.toolsList())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, name := range []string{
		godexSessionPromptWriteToolName, godexSessionPreemptToolName, godexSessionOutputReadToolName,
	} {
		if !strings.Contains(text, name) {
			t.Fatalf("full tool list missing %q: %s", name, text)
		}
	}
	for _, name := range []string{
		legacySessionPromptWriteToolName, legacySessionPreemptToolName, legacySessionOutputReadToolName,
	} {
		if strings.Contains(text, name) {
			t.Fatalf("legacy session tool leaked into advertisement: %s", text)
		}
	}

	execOnly := &execMCPHandler{mode: "exec", displayName: "repo", instanceID: "gdxi-test"}
	execEncoded, _ := json.Marshal(execOnly.toolsList())
	for _, name := range []string{
		godexSessionPromptWriteToolName, godexSessionPreemptToolName, godexSessionOutputReadToolName,
	} {
		if strings.Contains(string(execEncoded), name) {
			t.Fatalf("exec mode advertised session tool %q: %s", name, execEncoded)
		}
	}
}

func TestProdex04356SystemSessionQueueReadsStateDatabaseReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state_5.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)"); err != nil {
		t.Fatal(err)
	}
	const threadID = "00000000-0000-4000-8000-000000000123"
	if _, err := database.Exec("INSERT INTO threads(id, rollout_path) VALUES (?, ?)", threadID, "sessions/rollout.jsonl"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	queue := systemSessionQueueControl{}
	found, err := queue.persistedThread(path, threadID)
	if err != nil || !found {
		t.Fatalf("persisted thread = %t, %v", found, err)
	}
	rollout, err := queue.rolloutPath(path, threadID)
	if err != nil || rollout != "sessions/rollout.jsonl" {
		t.Fatalf("rollout path = %q, %v", rollout, err)
	}
}

func TestProdex04356SessionProtocolDispatchUsesGodexCanonicalSurfaceAndLegacyIngress(t *testing.T) {
	fixture := newSessionResolverFixture(t)
	rolloutDir := filepath.Join(fixture.codexHome, "sessions")
	if err := os.MkdirAll(rolloutDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(rolloutDir, "rollout-"+fixture.threadID+".jsonl")
	if err := os.WriteFile(rollout, []byte("{\"timestamp\":\"t1\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"ready\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := fakeSessionQueue{
		rollout: rollout,
		invocation: queueInvocation{
			outcome: queueAccepted, queued: true,
			messageID: "22222222-2222-4222-8222-222222222222",
		},
	}
	service := newExistingSessionService(fixture.process, queue)
	handler := &execMCPHandler{
		mode: "full", instanceID: "gdxi-test", workspace: fixture.workspace, sessions: service,
	}
	result, err := handler.callTool(t.Context(), legacySessionPromptWriteToolName, map[string]any{
		"message": "continue", "thread_id": fixture.threadID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "written" || result["message_id"] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("legacy prompt-write ingress = %#v", result)
	}
	read, err := handler.callTool(t.Context(), godexSessionOutputReadToolName, map[string]any{
		"thread_id": fixture.threadID, "limit": json.Number("10"), "wait_ms": json.Number("0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	events := read["events"].([]map[string]any)
	if len(events) != 1 || events[0]["kind"] != "assistant" || events[0]["text"] != "ready" {
		t.Fatalf("canonical output-read = %#v", read)
	}
	if err := validateToolArguments(godexSessionPromptWriteToolName, map[string]any{
		"message": "x", "unknown": true,
	}); err == nil || !strings.Contains(err.Error(), "unknown tool argument") {
		t.Fatalf("unknown session argument = %v", err)
	}
	execOnly := &execMCPHandler{
		mode: "exec", instanceID: "gdxi-test", workspace: fixture.workspace, sessions: service,
	}
	if _, err := execOnly.callTool(t.Context(), godexSessionOutputReadToolName, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "not exposed") {
		t.Fatalf("exec-only session tool = %v", err)
	}
}
