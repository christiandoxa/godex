package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"github.com/klauspost/compress/zstd"
)

type compressedRecoveryProcess04360 struct {
	path  string
	calls int
}

func (process *compressedRecoveryProcess04360) Run(_ context.Context, _ string, _ []string) error {
	process.calls++
	if process.calls == 1 {
		baseline, ok := readCompressedRecovery04360(process.path, recoveryCompressedBaselineCap04360)
		if !ok {
			return errors.New("baseline unavailable")
		}
		added := []byte(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n")
		encoder, err := zstd.NewWriter(nil)
		if err != nil {
			return err
		}
		encoded := encoder.EncodeAll(append(baseline, added...), nil)
		encoder.Close()
		next := process.path + ".tmp"
		if err = os.WriteFile(next, encoded, 0600); err != nil {
			return err
		}
		if err = os.Rename(next, process.path); err != nil {
			return err
		}
		return errors.New("synthetic usage limit child exit")
	}
	return nil
}

func TestProdex04360CompressedKnownSessionRecoveryExecutesOneContinuation(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	home := t.TempDir()
	path := filepath.Join(home, "rollout-"+id+".jsonl.zst")
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	baseline := encoder.EncodeAll([]byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), nil)
	encoder.Close()
	if err = os.WriteFile(path, baseline, 0600); err != nil {
		t.Fatal(err)
	}
	process := &compressedRecoveryProcess04360{path: path}
	accounts := &runPolicyAccounts{values: []accountentity.Account{
		{ID: "a", Name: "primary", Enabled: true}, {ID: "b", Name: "standby", Enabled: true},
	}, homes: map[string]string{"a": home, "b": home}}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	launcher := runSessionLauncher{
		runner:   runner,
		profiles: &verifiedRecoveryProfiles04360{fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{}, eligible: true},
	}
	released := 0
	err = launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
		ID: id, AccountID: "a", UpstreamAccountID: "a", ModelProvider: "openai", Path: path, CodexHome: home,
	}, []string{"exec", "resume", id, "old prompt"}, false,
		func(_ context.Context, session string) error {
			if session != id {
				t.Fatalf("wrong session binding %s", session)
			}
			released++
			return nil
		})
	if err != nil || released != 1 || process.calls != 2 {
		t.Fatalf("compressed continuation calls=%d released=%d err=%v", process.calls, released, err)
	}
}

func TestProdex04360CompletedGoalPreventsSessionRelaunch(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		status                 string
		wantCalls, wantRelease int
	}{
		{"completed", 1, 0},
		{"active", 2, 1},
	} {
		t.Run(tc.status, func(t *testing.T) {
			home := t.TempDir()
			fixtureGoalDB04360(t, home, id, tc.status)
			path := filepath.Join(home, "rollout-"+id+".jsonl")
			if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := &verifiedRecoveryProcess04360{file: path, mark: true}
			accounts := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "primary", Enabled: true}, {ID: "b", Name: "standby", Enabled: true},
			}, homes: map[string]string{"a": home, "b": home}}
			launcher := runSessionLauncher{
				runner:   runtimeusecase.NewRunner(accounts, process, nil),
				profiles: &verifiedRecoveryProfiles04360{fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{}, eligible: true},
			}
			release := 0
			err := launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
				ID: id, Path: path, CodexHome: home, AccountID: "a", UpstreamAccountID: "a", ModelProvider: "openai",
			}, []string{"exec", "resume", id, "old prompt"}, false,
				func(context.Context, string) error { release++; return nil })
			if len(process.calls) != tc.wantCalls || release != tc.wantRelease {
				t.Fatalf("status=%s calls=%d release=%d err=%v", tc.status, len(process.calls), release, err)
			}
		})
	}
}
