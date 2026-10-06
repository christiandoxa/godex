package superexpose

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
}

func (fake fakeSessionQueue) checkCapability(resolvedSessionTarget) error  { return fake.capabilityErr }
func (fake fakeSessionQueue) persistedThread(string, string) (bool, error) { return true, nil }
func (fake fakeSessionQueue) rolloutPath(string, string) (string, error)   { return "", nil }
func (fake fakeSessionQueue) queueOnce(resolvedSessionTarget, string) queueInvocation {
	return queueInvocation{outcome: queueAccepted, queued: true}
}
func (fake fakeSessionQueue) preempt(resolvedSessionTarget) (queuePreemptResult, error) {
	return queuePreemptResult{}, sessionQueueUnsupported
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
