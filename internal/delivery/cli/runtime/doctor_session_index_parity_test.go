package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type sessionIndexDoctorAccounts struct{ root string }

func (sessionIndexDoctorAccounts) Prepare() error        { return nil }
func (accounts sessionIndexDoctorAccounts) Root() string { return accounts.root }
func (sessionIndexDoctorAccounts) List(context.Context) ([]accountentity.Account, error) {
	return nil, nil
}

type sessionIndexCodexHome string

func (home sessionIndexCodexHome) CurrentCodexHome(context.Context) (string, error) {
	return string(home), nil
}

type sessionIndexFailingDoctor struct {
	repaired bool
	err      error
}

func (doctor *sessionIndexFailingDoctor) RepairSessionIndex(context.Context) error {
	doctor.repaired = true
	return nil
}
func (doctor *sessionIndexFailingDoctor) Diagnose(context.Context, runtimemodel.DoctorOptions) (runtimemodel.DoctorDiagnostics, error) {
	return runtimemodel.DoctorDiagnostics{}, doctor.err
}
func (*sessionIndexFailingDoctor) SaveBundle(string, []byte) (string, error) { return "", nil }

func TestDoctorSessionIndexRepairMatchesProdexOrderingAndProductionPath(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("integration helper uses a POSIX shell")
	}
	root := t.TempDir()
	activeHome := filepath.Join(root, "active-codex")
	godexHome := filepath.Join(root, "godex")
	sessionID := "01900000-0000-7000-8000-000000000051"
	session := filepath.Join(activeHome, "sessions", "2026", "10", "03", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(root, "deleted-overlay")
	oldAttachment := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(oldAttachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAttachment, []byte("doctor attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"{\"timestamp\":\"2026-10-03T12:00:00Z\",\"type\":\"event\",\"payload\":{\"message\":\"read " + oldAttachment + "\"}}",
		"{\"timestamp\":\"2026-10-03T12:01:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"" + sessionID + "\",\"thread_id\":\"thread-1\"}}",
	}, "\n") + "\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	stableAttachment := filepath.Join(activeHome, "attachments", "thread-1", "pasted-text-1.txt")
	record := filepath.Join(root, "child-environment")
	script := filepath.Join(root, "codex")
	shell := strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = --version ]; then printf '%s\\n' 'codex 0.160.0'; exit 0; fi",
		"for argument in \"$@\"; do [ \"$argument\" = exec-server ] && exit 0; done",
		"[ \"$1\" = app-server ] || exit 42",
		"head -n 1 \"$GODEX_DOCTOR_SESSION\" | grep -q '\"type\":\"session_meta\"' || exit 43",
		"[ -f \"$GODEX_DOCTOR_STABLE_ATTACHMENT\" ] || exit 44",
		"printf '%s|%s' \"$CODEX_HOME\" \"$CODEX_SQLITE_HOME\" > \"$GODEX_DOCTOR_REPAIR_RECORD\"",
		"read line",
		"printf '%s\\n' '{\"id\":1,\"result\":{}}'",
		"read line",
		"read line",
		"printf '%s\\n' '{\"id\":2,\"result\":{\"nextCursor\":null}}'",
		"read line",
		"printf '%s\\n' '{\"id\":3,\"result\":{\"nextCursor\":null}}'",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(shell), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_DOCTOR_REPAIR_RECORD", record)
	t.Setenv("GODEX_DOCTOR_SESSION", session)
	t.Setenv("GODEX_DOCTOR_STABLE_ATTACHMENT", stableAttachment)
	t.Setenv("PRODEX_RUNTIME_TIMINGS", "")

	var timing strings.Builder
	process := codex.NewCodexProcess(script, codex.Terminal{Stderr: &timing})
	doctor := runtimeusecase.NewDoctor(sessionIndexDoctorAccounts{root: godexHome}, process)
	doctor.SetSessionIndexRepairer(process)
	doctor.SetActiveCodexHomeResolver(sessionIndexCodexHome(activeHome))
	doctor.SetSharedCodexHome(activeHome)

	var output, diagnostic strings.Builder
	if err := DoctorWithErrorOutput(t.Context(), doctor, &output, &diagnostic, []string{"--repair-session-index"}); err != nil {
		t.Fatal(err)
	}
	if diagnostic.String() != "godex doctor: session index repair completed.\n" {
		t.Fatalf("repair notice = %q", diagnostic.String())
	}
	if !strings.Contains(output.String(), "Doctor\n") {
		t.Fatalf("doctor output = %q", output.String())
	}
	if got, err := os.ReadFile(record); err != nil || string(got) != activeHome+"|"+activeHome {
		t.Fatalf("app-server environment = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(stableAttachment); err != nil || string(got) != "doctor attachment" {
		t.Fatalf("stable attachment = %q, err=%v", got, err)
	}
	repaired, err := os.ReadFile(session)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Split(strings.TrimSpace(string(repaired)), "\n")[0]
	if !strings.Contains(first, "\"type\":\"session_meta\"") {
		t.Fatalf("metadata was not repaired before app-server launch: %q", first)
	}
	if _, err := os.Stat(session + ".prodex-repair-bak"); err != nil {
		t.Fatalf("metadata backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(godexHome, "shared-codex-session-maintenance-v1.json")); err != nil {
		t.Fatalf("maintenance cache missing from Godex home: %v", err)
	}
	if !strings.Contains(timing.String(), "prodex_runtime_timing stage=startup.thread_index_reconcile_ms duration_ms=") {
		t.Fatalf("runtime timing = %q", timing.String())
	}
}

func TestDoctorSessionIndexRepairNoticePrecedesLaterDiagnosticFailure(t *testing.T) {
	want := errors.New("synthetic diagnostics failure")
	doctor := &sessionIndexFailingDoctor{err: want}
	var output, diagnostic strings.Builder
	err := DoctorWithErrorOutput(t.Context(), doctor, &output, &diagnostic, []string{"--repair-session-index"})
	if !errors.Is(err, want) || !doctor.repaired {
		t.Fatalf("error/repaired = %v/%t", err, doctor.repaired)
	}
	if diagnostic.String() != "godex doctor: session index repair completed.\n" {
		t.Fatalf("repair notice = %q", diagnostic.String())
	}
}

func TestDoctorUsageDocumentsSessionIndexRepair(t *testing.T) {
	if !strings.Contains(doctorUsage, "--repair-session-index") {
		t.Fatalf("doctor usage = %q", doctorUsage)
	}
}
