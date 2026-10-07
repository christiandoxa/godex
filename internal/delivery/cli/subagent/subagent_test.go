package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const helperModeEnv = "GODEX_SUBAGENT_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperModeEnv) {
	case "success":
		fmt.Printf("marker=%s launcher=%s", os.Getenv(recursionMarker), os.Getenv(launcherMarker))
		os.Exit(0)
	case "failure":
		fmt.Fprint(os.Stderr, "child-failed")
		os.Exit(23)
	case "silent":
		os.Exit(0)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProdex04356SubAgentChildArgvMatchesTaggedPlan(t *testing.T) {
	model, effort := "模型/β-🦀", "xhigh"
	spec := childLaunchSpec{
		Provider:        "copilot",
		Model:           &model,
		Effort:          &effort,
		PresidioEnabled: true,
		RequiredTools:   []string{"rtk", "ponytail"},
	}
	task := `spaces 'apostrophe' "quotes"
Unicode 任务; $(touch nope) & |`
	got := childArgv(spec, task)
	want := []string{
		"s", "--no-sub-agent", "--presidio",
		"--require-tool", "rtk",
		"--require-tool", "ponytail",
		"--provider", "copilot",
		"--model", model,
		"-c", "model_reasoning_effort=xhigh",
		"exec", task,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("child argv = %#v, want %#v", got, want)
	}
	if strings.Count(strings.Join(got, "\x00"), task) != 1 {
		t.Fatalf("task was not preserved as one argv value: %#v", got)
	}

	local := "http://127.0.0.1:11434/v1"
	localSpec := childLaunchSpec{Provider: "local", LocalURL: &local}
	if got := childArgv(localSpec, "task"); !reflect.DeepEqual(got,
		[]string{"s", "--no-sub-agent", "--no-presidio", "--url", local, "exec", "task"}) {
		t.Fatalf("local child argv = %#v", got)
	}
	openAI := childArgv(childLaunchSpec{Provider: "openai"}, "task")
	if !reflect.DeepEqual(openAI,
		[]string{"s", "--no-sub-agent", "--no-presidio", "-c", `model_provider="openai"`, "exec", "task"}) {
		t.Fatalf("openai child argv = %#v", openAI)
	}
}

func TestProdex04356SubAgentSpecValidationIsFailClosed(t *testing.T) {
	base := validSpec(t)
	cases := []struct {
		name string
		edit func(*childLaunchSpec)
		want string
	}{
		{"relative executable", func(spec *childLaunchSpec) { spec.Executable = "godex" }, "must be absolute"},
		{"marker", func(spec *childLaunchSpec) { spec.RecursionMarker = "OTHER" }, "recursion marker"},
		{"task limit", func(spec *childLaunchSpec) { spec.TaskMaxBytes = 65537 }, "task size policy"},
		{"concurrency", func(spec *childLaunchSpec) { spec.MaxConcurrency.Value = 65 }, "between 1 and 64"},
		{"source", func(spec *childLaunchSpec) { spec.MaxConcurrency.Source = "other" }, "source is invalid"},
		{"provider", func(spec *childLaunchSpec) { spec.Provider = "other" }, "invalid sub-agent provider"},
		{"non-local url", func(spec *childLaunchSpec) { u := "https://example.test"; spec.LocalURL = &u }, "valid only for the local provider"},
		{"tool", func(spec *childLaunchSpec) { spec.RequiredTools = []string{"unknown"} }, "unknown optional tool"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := base
			testCase.edit(&spec)
			if err := validateSpec(spec); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validation error = %v, want %q", err, testCase.want)
			}
		})
	}

	local := base
	local.Provider = "local"
	if err := validateSpec(local); err == nil || !strings.Contains(err.Error(), "requires a URL") {
		t.Fatalf("local missing URL = %v", err)
	}
	badURL := "https://user:pass@example.test/v1"
	local.LocalURL = &badURL
	if err := validateSpec(local); err == nil || !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("credential URL = %v", err)
	}
}

func TestProdex04356SubAgentConfigRejectsUnknownFields(t *testing.T) {
	spec := validSpec(t)
	content, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	content = bytes.TrimSuffix(content, []byte("}"))
	content = append(content, []byte(`,"unexpected":true}`)...)
	if _, err := decodeSpec(content); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown config field = %v", err)
	}
}

func TestProdex04356SubAgentRecursionGate(t *testing.T) {
	t.Setenv(recursionMarker, "1")
	t.Setenv(launcherMarker, "")
	err := run(t.Context(), arguments{configPath: "unused", taskFile: "unused"}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), recursionMarker) {
		t.Fatalf("recursion gate error = %v", err)
	}
}

func TestProdex04356SubAgentSlotLimitReturns75WithoutCreatingSlots(t *testing.T) {
	spec := validSpec(t)
	spec.MaxConcurrency.Value = 1
	slot := filepath.Join(spec.SlotDir, "slot-00.lock")
	release, err := lockfile.TryAcquireExisting(slot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, err = acquireSlot(spec)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 75 {
		t.Fatalf("slot saturation = %#v", err)
	}
	if _, err := os.Stat(filepath.Join(spec.SlotDir, "slot-01.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("launcher unexpectedly created extra slot: %v", err)
	}
}

func TestProdex04356SubAgentRunRelaysOutputDeletesTaskAndSanitizesMarkers(t *testing.T) {
	spec := validSpec(t)
	config, task := writeFixture(t, spec, "narrow task")
	t.Setenv(helperModeEnv, "success")
	t.Setenv(launcherMarker, "1")
	var stdout bytes.Buffer
	if err := RunArguments(t.Context(), []string{"--config", config, "--task-file", task}, &stdout, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "marker=1 launcher=" {
		t.Fatalf("child environment/output = %q", got)
	}
	if _, err := os.Stat(task); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed task still exists: %v", err)
	}
}

func TestProdex04356SubAgentRunPreservesChildExitCode(t *testing.T) {
	spec := validSpec(t)
	config, task := writeFixture(t, spec, "narrow task")
	t.Setenv(helperModeEnv, "failure")
	var stderr bytes.Buffer
	err := RunArguments(t.Context(), []string{"--config", config, "--task-file", task}, ioDiscard{}, &stderr)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("child exit = %#v", err)
	}
	if stderr.String() != "child-failed" {
		t.Fatalf("child stderr = %q", stderr.String())
	}
}

func TestProdex04356SubAgentRunRejectsSilentSuccess(t *testing.T) {
	spec := validSpec(t)
	config, task := writeFixture(t, spec, "narrow task")
	t.Setenv(helperModeEnv, "silent")
	err := RunArguments(t.Context(), []string{"--config", config, "--task-file", task}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "without output") {
		t.Fatalf("silent success = %v", err)
	}
}

func TestProdex04356SubAgentRunCancellationReturns130(t *testing.T) {
	spec := validSpec(t)
	config, task := writeFixture(t, spec, "narrow task")
	t.Setenv(helperModeEnv, "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := RunArguments(ctx, []string{"--config", config, "--task-file", task}, ioDiscard{}, ioDiscard{})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("cancel exit = %#v", err)
	}
}

func TestProdex04356SubAgentTaskMustBeDirectChild(t *testing.T) {
	spec := validSpec(t)
	configBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(config, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "task.txt")
	if err := os.WriteFile(outside, []byte("task"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = RunArguments(t.Context(), []string{"--config", config, "--task-file", outside}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "directly inside") {
		t.Fatalf("outside task = %v", err)
	}
}

func validSpec(t *testing.T) childLaunchSpec {
	t.Helper()
	root := t.TempDir()
	slotDir := filepath.Join(root, "slots")
	taskDir := filepath.Join(root, "tasks")
	if err := os.MkdirAll(slotDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(taskDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slotDir, "slot-00.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return childLaunchSpec{
		Executable: executable,
		Provider:   "openai",
		MaxConcurrency: maxConcurrency{
			Value:  1,
			Source: "custom",
		},
		SlotDir:         slotDir,
		TaskDir:         taskDir,
		TaskMaxBytes:    65_536,
		RecursionMarker: recursionMarker,
	}
}

func writeFixture(t *testing.T, spec childLaunchSpec, taskContent string) (string, string) {
	t.Helper()
	config := filepath.Join(t.TempDir(), "sub-agent-launch.json")
	content, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, content, 0o600); err != nil {
		t.Fatal(err)
	}
	task := filepath.Join(spec.TaskDir, "task.txt")
	if err := os.WriteFile(task, []byte(taskContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return config, task
}

type ioDiscard struct{}

func (ioDiscard) Write(content []byte) (int, error) { return len(content), nil }

func TestProdex04356SubAgentFailedSpawnPreservesTaskAndReleasesSlot(t *testing.T) {
	spec := validSpec(t)
	spec.Executable = filepath.Join(t.TempDir(), "missing-godex-binary")
	config, task := writeFixture(t, spec, "narrow task")
	err := RunArguments(t.Context(), []string{"--config", config, "--task-file", task}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "failed to spawn sub-agent child") {
		t.Fatalf("failed spawn error = %v", err)
	}
	if _, err := os.Stat(task); err != nil {
		t.Fatalf("failed spawn removed retryable task: %v", err)
	}
	release, err := acquireSlot(spec)
	if err != nil {
		t.Fatalf("failed spawn leaked slot: %v", err)
	}
	_ = release()
}

func TestProdex04356SubAgentLimitSecuresAndPreservesTask(t *testing.T) {
	spec := validSpec(t)
	config, task := writeFixture(t, spec, "narrow task")
	if err := os.Chmod(task, 0o666); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireSlot(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer lease()
	err = RunArguments(t.Context(), []string{"--config", config, "--task-file", task}, ioDiscard{}, ioDiscard{})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 75 {
		t.Fatalf("limit error = %#v", err)
	}
	info, statErr := os.Stat(task)
	if statErr != nil {
		t.Fatalf("limit removed retryable task: %v", statErr)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("retryable task mode = %o, want 600", got)
		}
	}
}

func TestProdex04356SubAgentBoundedUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded.txt")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, 65_537), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedUTF8(path, 65_536, "fixture"); err == nil || !strings.Contains(err.Error(), "65536-byte limit") {
		t.Fatalf("oversized file = %v", err)
	}
	if err := os.WriteFile(path, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedUTF8(path, 65_536, "fixture"); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid UTF-8 = %v", err)
	}
}
