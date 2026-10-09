package runtime

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type sessionMaintenanceProcess struct {
	fakeProcess
	shared string
	cache  string
}

func (process *sessionMaintenanceProcess) MaintainSessions(shared, cache string) error {
	process.shared, process.cache = shared, cache
	return nil
}

func TestResumedSessionRunsSharedMaintenanceBeforeLaunch(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "home", Enabled: true}},
		homes:    map[string]string{"home": "/profiles/home"},
	}
	process := &sessionMaintenanceProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetSharedCodexHome("/shared/codex")
	runner.SetManagedProfilesRoot("/godex/profiles")

	if err := runner.RunSession(context.Background(), "home", "home", []string{"resume", "session"}); err != nil {
		t.Fatal(err)
	}
	if process.shared != "/shared/codex" || process.cache != "/godex" {
		t.Fatalf("maintenance roots = %q/%q", process.shared, process.cache)
	}
	if len(process.homes) != 1 || process.homes[0] != "/profiles/home" {
		t.Fatalf("launch home = %#v", process.homes)
	}
}
