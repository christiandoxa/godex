package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type antigravityProcessFake struct {
	arguments    []string
	codexHome    string
	preparedHome string
	runs         int
	err          error
	prepareErr   error
}

func (process *antigravityProcessFake) PrepareCodexHome(codexHome string) error {
	process.preparedHome = codexHome
	return process.prepareErr
}

func (process *antigravityProcessFake) RunRuntimeWithCodexHome(_ context.Context, codexHome string, arguments []string) error {
	process.runs++
	process.codexHome = codexHome
	process.arguments = append([]string(nil), arguments...)
	return process.err
}

type antigravitySessionLockerFake struct {
	home       string
	released   bool
	lockErr    error
	releaseErr error
}

func (locker *antigravitySessionLockerFake) LockCodexSessionsForChild(_ context.Context, home string) (func() error, error) {
	locker.home = home
	if locker.lockErr != nil {
		return nil, locker.lockErr
	}
	return func() error {
		locker.released = true
		return locker.releaseErr
	}, nil
}

func TestPrepareAntigravityCodexHomeUsesConfiguredGateway(t *testing.T) {
	process := &antigravityProcessFake{}
	runner := &Runner{antigravity: process, antigravityHome: "/synthetic/shared-codex"}
	if err := runner.PrepareAntigravityCodexHome(); err != nil {
		t.Fatal(err)
	}
	if process.preparedHome != "/synthetic/shared-codex" {
		t.Fatalf("prepared Antigravity CODEX_HOME = %q", process.preparedHome)
	}
	process.prepareErr = errors.New("synthetic prepare failure")
	if err := runner.PrepareAntigravityCodexHome(); !errors.Is(err, process.prepareErr) {
		t.Fatalf("prepare error = %v", err)
	}
}

func TestRunAntigravityAddsNativeFlagsAndPreservesUserOverrides(t *testing.T) {
	process := &antigravityProcessFake{}
	runner := &Runner{antigravity: process}
	runner.SetAntigravityCodexHome("/synthetic/shared-codex")
	locker := &antigravitySessionLockerFake{}
	runner.SetAntigravitySessionLocker(locker)
	userArguments := []string{"exec", "review"}
	if err := runner.RunAntigravity(context.Background(), " gemini-3.1-pro ", userArguments); err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", " gemini-3.1-pro ", "--dangerously-skip-permissions", "exec", "review"}
	if !reflect.DeepEqual(process.arguments, want) || !reflect.DeepEqual(userArguments, []string{"exec", "review"}) {
		t.Fatalf("Antigravity args = %#v, user args = %#v", process.arguments, userArguments)
	}
	if process.codexHome != "/synthetic/shared-codex" {
		t.Fatalf("Antigravity CODEX_HOME = %q", process.codexHome)
	}
	if locker.home != process.codexHome || !locker.released {
		t.Fatalf("session lock home/released = %q/%t", locker.home, locker.released)
	}

	process.arguments = nil
	userArguments = []string{"--dangerously-skip-permissions", "-m=custom", "exec"}
	locker.released = false
	if err := runner.RunAntigravity(context.Background(), "ignored", userArguments); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(process.arguments, userArguments) || !locker.released {
		t.Fatalf("explicit native args = %#v, want %#v", process.arguments, userArguments)
	}
	for _, modelArguments := range [][]string{
		{"--model", "native"}, {"--model=native"}, {"-m", "native"}, {"-m=native"}, {"--model"},
	} {
		got := antigravityArguments("ignored", modelArguments)
		want := append([]string{"--dangerously-skip-permissions"}, modelArguments...)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("model override %#v became %#v, want %#v", modelArguments, got, want)
		}
	}
}

func TestRunAntigravityReportsMissingProcessAndChildFailure(t *testing.T) {
	if err := (&Runner{}).RunAntigravity(context.Background(), "", nil); err == nil {
		t.Fatal("missing Antigravity process unexpectedly accepted")
	}
	if err := (&Runner{antigravity: &antigravityProcessFake{}}).RunAntigravity(context.Background(), "", nil); err == nil {
		t.Fatal("missing Antigravity CODEX_HOME unexpectedly accepted")
	}
	childErr := errors.New("synthetic child failure")
	process := &antigravityProcessFake{err: childErr}
	locker := &antigravitySessionLockerFake{}
	runner := &Runner{antigravity: process, antigravityHome: "/synthetic/shared-codex", sessionLocker: locker}
	if err := runner.RunAntigravity(context.Background(), "", nil); !errors.Is(err, childErr) {
		t.Fatalf("child error = %v", err)
	}
	if !locker.released {
		t.Fatal("session lock was not released after child failure")
	}
	if err := (&Runner{antigravity: process, antigravityHome: "/synthetic/shared-codex"}).RunAntigravity(context.Background(), "", nil); err == nil {
		t.Fatal("missing Antigravity session locker unexpectedly accepted")
	}
	lockErr := errors.New("synthetic lock failure")
	locker = &antigravitySessionLockerFake{lockErr: lockErr}
	runner.sessionLocker = locker
	if err := runner.RunAntigravity(context.Background(), "", nil); !errors.Is(err, lockErr) || process.runs != 1 {
		t.Fatalf("lock failure = %v, child runs = %d", err, process.runs)
	}
}
