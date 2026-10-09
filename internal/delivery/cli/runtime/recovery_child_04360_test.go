package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type verifiedRecoveryProfiles04360 struct {
	*fakeLocalLaunchProfiles
	eligible bool
}

func (profiles *verifiedRecoveryProfiles04360) ResolveLaunch(_ context.Context, name string) (profilemodel.LaunchTarget, error) {
	if name != "standby" {
		return profilemodel.LaunchTarget{}, errors.New("profile unavailable")
	}
	auth := "no-auth"
	if profiles.eligible {
		auth = "chatgpt"
	}
	return profilemodel.LaunchTarget{Name: "standby", CodexHome: "/synthetic/standby", Provider: "openai", AccountID: "b", Auth: auth}, nil
}
func (profiles *verifiedRecoveryProfiles04360) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return []sessionmodel.ProfileHome{
		{Name: "primary", AccountID: "a", Provider: "openai", Enabled: true},
		{Name: "standby", AccountID: "b", Provider: "openai", Enabled: true},
	}, nil
}

type verifiedRecoveryProcess04360 struct {
	file      string
	calls     [][]string
	homes     []string
	mark      bool
	cancelled bool
}

func (process *verifiedRecoveryProcess04360) Run(_ context.Context, home string, args []string) error {
	process.calls = append(process.calls, append([]string(nil), args...))
	process.homes = append(process.homes, home)
	if len(process.calls) == 1 {
		if process.mark {
			f, err := os.OpenFile(process.file, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, err = f.WriteString(
				`{"type":"response_item","payload":{"type":"message","role":"user","content":[]}}` + "\n" +
					`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n")
			if err != nil {
				_ = f.Close()
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
		}
		if process.cancelled {
			return recoveryCancelled04360{}
		}
		return errors.New("synthetic child exit")
	}
	return nil
}

type recoveryCancelled04360 struct{}

func (recoveryCancelled04360) Error() string { return "user interrupted" }
func (recoveryCancelled04360) ExitCode() int { return 130 }

func TestProdex04360KnownSessionChildExitSafelyRelauchesOnce(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name                               string
		marker, authenticated, allowRotate bool
		cancelled                          bool
		releaseFails                       bool
		wantCalls, wantForget              int
		expectErr                          bool
	}{
		{"new accepted signal and ready other profile", true, true, true, false, false, 2, 1, false},
		{"binding-release failure prevents relaunch", true, true, true, false, true, 1, 1, true},
		{"interrupted child must not relaunch", true, true, true, true, false, 1, 0, true},
		{"no post-child signal", false, true, true, false, false, 1, 0, true},
		{"fallback not authenticated", true, false, true, false, false, 1, 0, true},
		{"explicit no-auto-rotate", true, true, false, false, false, 1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			sessionFile := filepath.Join(home, "rollout-"+session+".jsonl")
			if err := os.WriteFile(sessionFile, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := &verifiedRecoveryProcess04360{file: sessionFile, mark: tc.marker, cancelled: tc.cancelled}
			accountSource := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "primary", Enabled: true},
				{ID: "b", Name: "standby", Enabled: true},
			}, homes: map[string]string{"a": home, "b": home}}
			runner := runtimeusecase.NewRunner(accountSource, process, nil)
			eligibleProfiles := &verifiedRecoveryProfiles04360{
				fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{active: false},
				eligible:                tc.authenticated,
			}
			allowRotate := tc.allowRotate
			launcher := runSessionLauncher{
				runner: runner, profiles: eligibleProfiles,
				options: runtimeusecase.RuntimeLaunchOptions{AllowAutoRotate: &allowRotate},
			}
			report := sessionmodel.Report{
				ID: session, Path: sessionFile, AccountID: "a", UpstreamAccountID: "a",
				ModelProvider: "openai", LastModel: "gpt-6.1-sol", LastReasoningEffort: "ultra",
			}
			forgotten := 0
			forget := func(_ context.Context, id string) error {
				if id != session {
					t.Fatal("forgot wrong session")
				}
				forgotten++
				if len(process.calls) != 1 {
					t.Fatal("binding release did not precede second launch")
				}
				if tc.releaseFails {
					return errors.New("binding store unavailable")
				}
				return nil
			}
			original := []string{"exec", "--cyber-access-program", "daybreak_blue", "resume", session, "old prompt"}
			err := launcher.RunSessionReportWithRecovery(t.Context(), report, original, false, forget)
			if (err != nil) != tc.expectErr || len(process.calls) != tc.wantCalls || forgotten != tc.wantForget {
				t.Fatalf("calls %d forgotten %d err %v", len(process.calls), forgotten, err)
			}
			if len(process.calls) == 2 {
				retarget := process.calls[1]
				for _, want := range []string{
					"resume", session, "--cyber-access-program", "daybreak_blue",
					`model="gpt-6.1-sol"`, `model_reasoning_effort="ultra"`,
					recoveryContinuationPrompt04360,
				} {
					if !slices.Contains(retarget, want) {
						t.Fatalf("lost verified recovery field %q: %#v", want, retarget)
					}
				}
				if slices.Contains(retarget, "old prompt") {
					t.Fatalf("unsafe prompt replay: %#v", retarget)
				}
				if process.homes[1] != home {
					t.Fatalf("recovery launched wrong home: %#v", process.homes)
				}
				idIdx := slices.Index(retarget, session)
				if idIdx < 0 || idIdx == 0 || retarget[idIdx-1] != "resume" {
					t.Fatalf("retarget missed normalized exec resume session: %#v", retarget)
				}
			}
		})
	}
}

func TestProdex04360NoBindingReleaseMeansNoAutomaticRetry(t *testing.T) {
	report := sessionmodel.Report{ID: target04360, Path: filepath.Join(t.TempDir(), "nonexistent.jsonl"), AccountID: "a", UpstreamAccountID: "a"}
	launcher := runSessionLauncher{}
	if launcher.recoveryEligible04360(report, []string{"exec", "resume", target04360}, false, nil) {
		t.Fatal("unbound recovery must not proceed")
	}
	if launcher.recoveryEligible04360(report, []string{"resume", target04360}, false, nil) {
		t.Fatal("unbound TUI recovery must not proceed")
	}
}

func TestProdex04361KnownTUIGoalResumeUsesVerifiedRecoveryLifecycle(t *testing.T) {
	const session = target04360
	home := t.TempDir()
	fixtureGoalDB04360(t, home, session, "active")
	sessionFile := filepath.Join(home, "rollout-"+session+".jsonl")
	if err := os.WriteFile(sessionFile,
		[]byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	process := &verifiedRecoveryProcess04360{file: sessionFile, mark: true}
	accounts := &runPolicyAccounts{values: []accountentity.Account{
		{ID: "a", Name: "primary", Enabled: true},
		{ID: "b", Name: "standby", Enabled: true},
	}, homes: map[string]string{"a": home, "b": home}}
	profiles := &verifiedRecoveryProfiles04360{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{active: false}, eligible: true,
	}
	allowRotate := true
	launcher := runSessionLauncher{
		runner: runtimeusecase.NewRunner(accounts, process, nil), profiles: profiles,
		options: runtimeusecase.RuntimeLaunchOptions{AllowAutoRotate: &allowRotate},
	}
	released := 0
	err := launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
		ID: session, Path: sessionFile, CodexHome: home,
		AccountID: "a", UpstreamAccountID: "a", ModelProvider: "openai",
		LastReasoningEffort: "ultra",
	}, []string{"--model", "gpt-6.1-sol", "resume", session, "old prompt", "--no-alt-screen"}, false,
		func(_ context.Context, id string) error {
			if id != session || len(process.calls) != 1 {
				t.Fatalf("affinity release id/call count = %q/%d", id, len(process.calls))
			}
			released++
			return nil
		})
	if err != nil || len(process.calls) != 2 || released != 1 {
		t.Fatalf("TUI goal recovery calls/release/error = %d/%d/%v", len(process.calls), released, err)
	}
	retarget := process.calls[1]
	for _, want := range []string{"--model", "gpt-6.1-sol", "resume", session, "--no-alt-screen", `model_reasoning_effort="ultra"`, "/goal resume"} {
		if !slices.Contains(retarget, want) {
			t.Fatalf("TUI recovery lost %q: %#v", want, retarget)
		}
	}
	if slices.Contains(retarget, "old prompt") || slices.Contains(retarget, recoveryContinuationPrompt04360) {
		t.Fatalf("TUI goal recovery replayed prompt or used non-goal continuation: %#v", retarget)
	}
}

func TestProdex04360RecoveryProfilerNeverPicksFailedOrUnreadyAccount(t *testing.T) {
	accounts := &verifiedRecoveryProfiles04360{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{},
		eligible:                true,
	}
	launcher := runSessionLauncher{profiles: accounts}
	candidate, ok := launcher.recoveryCandidate04360(context.Background(), sessionmodel.Report{AccountID: "a", UpstreamAccountID: "a"})
	if !ok || !reflect.DeepEqual(candidate.AccountID, "b") {
		t.Fatalf("wrong recovery candidate %#v %t", candidate, ok)
	}
	accounts.eligible = false
	_, ok = launcher.recoveryCandidate04360(context.Background(), sessionmodel.Report{AccountID: "a", UpstreamAccountID: "a"})
	if ok {
		t.Fatal("unready profile selected")
	}
}

func TestProdex04360RecoveryResolvesAccountNameBeforeExclusion(t *testing.T) {
	profiles := &verifiedRecoveryProfiles04360{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{},
		eligible:                true,
	}
	launcher := runSessionLauncher{profiles: profiles}
	candidate, ok := launcher.recoveryCandidate04360(context.Background(), sessionmodel.Report{
		AccountID: "primary", UpstreamAccountID: "primary",
	})
	if !ok || candidate.AccountID != "b" {
		t.Fatalf("alias recovery candidate=%#v ok=%t", candidate, ok)
	}
}

// Retained to guard output stability for copies of previous data; no
// source-specific values may be rewritten except the session target.
func TestProdex04360RecoveryOptionsContainNoInvisibleModelDefault(t *testing.T) {
	got, ok := retargetCodexExecRecovery04360([]string{"exec", "hello"}, target04360)
	if !ok || strings.Contains(strings.Join(got, " "), "model=") {
		t.Fatalf("model default inserted in recovery: %#v", got)
	}
}
