package runtime

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"

	_ "modernc.org/sqlite"
)

type goalOnlyRecoveryProcess04360 struct {
	dbPath  string
	calls   int
	changed bool
}

func (process *goalOnlyRecoveryProcess04360) Run(_ context.Context, _ string, _ []string) error {
	process.calls++
	if process.calls == 1 {
		if process.changed {
			db, err := sql.Open("sqlite", process.dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if _, err = db.Exec("UPDATE thread_goals SET status='usage_limited',updated_at_ms=101"); err != nil {
				return err
			}
		}
		return errors.New("synthetic failed child")
	}
	return nil
}
func TestProdex04360GoalTransitionRelauchesKnownSessionWithoutFalseTextMarker(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name   string
		change bool
		want   int
	}{
		{"actual goal transition", true, 2},
		{"old active goal without new limit", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dbPath := fixtureGoalDB04360(t, home, id, "active")
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("UPDATE thread_goals SET updated_at_ms=100"); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			rollout := filepath.Join(home, "rollout-"+id+".jsonl")
			if err = os.WriteFile(rollout, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := &goalOnlyRecoveryProcess04360{dbPath: dbPath, changed: tc.change}
			accounts := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "primary", Enabled: true}, {ID: "b", Name: "standby", Enabled: true},
			}, homes: map[string]string{"a": home, "b": home}}
			launcher := runSessionLauncher{
				runner:   runtimeusecase.NewRunner(accounts, process, nil),
				profiles: &verifiedRecoveryProfiles04360{fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{}, eligible: true},
			}
			forgot := 0
			err = launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
				ID: id, Path: rollout, CodexHome: home, AccountID: "a", UpstreamAccountID: "a", ModelProvider: "openai",
			}, []string{"exec", "resume", id, "old prompt"}, false,
				func(context.Context, string) error { forgot++; return nil })
			if process.calls != tc.want || forgot != tc.want-1 {
				t.Fatalf("goal transition calls=%d forget=%d err=%v", process.calls, forgot, err)
			}
		})
	}
}
