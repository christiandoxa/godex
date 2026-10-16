package runtime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type failedFallbackProfiles04380 struct {
	*multiProfileRecoverySource04360
}

func (s *failedFallbackProfiles04380) ResolveLaunch(ctx context.Context, name string) (profilemodel.LaunchTarget, error) {
	if name == "first" {
		return profilemodel.LaunchTarget{
			Name: "first", AccountID: "a", Provider: "openai", Auth: "chatgpt", CodexHome: "/synthetic/first",
		}, nil
	}
	return s.multiProfileRecoverySource04360.ResolveLaunch(ctx, name)
}

type failedFallbackQuota04380 struct{ ready map[string]bool }

func (q *failedFallbackQuota04380) Ready(_ context.Context, account accountentity.Account) (bool, error) {
	return q.ready[account.ID], nil
}

type failedFallbackProcess04380 struct {
	*sequentialRecoveryProcess04360
	quota                 *failedFallbackQuota04380
	backupReadyAfterRound bool
}

func (p *failedFallbackProcess04380) Run(ctx context.Context, home string, args []string) error {
	err := p.sequentialRecoveryProcess04360.Run(ctx, home, args)
	if len(p.calls) == 3 && p.quota != nil {
		// After B and C actually hit a fresh rate-limit, a new observation
		// after the bounded retry wait finds only the original A ready.
		p.quota.ready["b"] = p.backupReadyAfterRound
		p.quota.ready["c"] = false
	}
	return err
}

// Prodex 0.438.0 run_command_strategy.rs and Mojo
// usage_limit_recovery_candidate: after B,C are attempted and a transient
// retry wait completes, A can be selected one more time ONLY if its own
// quota has recovered. No original prompt/tool output may be replayed.
func TestProdex04380SessionRecoveryFailedProfileFallbackAfterPoolRetry(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name                  string
		originalReady         bool
		skipQuota             bool
		withQuota             bool
		allowWait             bool
		wantCalls             int
		wantLast              string
		backupReadyAfterRound bool
	}{
		{name: "verified recovered original after B and C", originalReady: true, withQuota: true, allowWait: true, wantCalls: 4, wantLast: "a"},
		{name: "original still exhausted fails closed", originalReady: false, withQuota: true, allowWait: true, wantCalls: 3},
		{name: "without quota reader only backups recycle, not original", originalReady: true, withQuota: false, allowWait: true, wantCalls: 4, wantLast: "b"},
		{name: "cancelled transient wait prevents fallback", originalReady: true, withQuota: true, allowWait: false, wantCalls: 3},
		{name: "explicit skip quota preserves backup precedence", originalReady: false, skipQuota: true, withQuota: true, allowWait: true, wantCalls: 4, wantLast: "b"},
		{name: "ready backup beats recovered original", originalReady: true, withQuota: true, allowWait: true, wantCalls: 4, wantLast: "b", backupReadyAfterRound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			rollout := filepath.Join(home, "rollout-"+session+".jsonl")
			if err := os.WriteFile(rollout, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			quota := &failedFallbackQuota04380{ready: map[string]bool{"a": tc.originalReady, "b": true, "c": true}}
			process := &failedFallbackProcess04380{sequentialRecoveryProcess04360: &sequentialRecoveryProcess04360{
				path: rollout, failUntil: 3, emit: map[int]bool{1: true, 2: true, 3: true}}, quota: quota,
				backupReadyAfterRound: tc.backupReadyAfterRound}
			accounts := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "first", Enabled: true}, {ID: "b", Name: "second", Enabled: true}, {ID: "c", Name: "third", Enabled: true},
			}, homes: map[string]string{"a": home, "b": filepath.Join(home, "second"), "c": filepath.Join(home, "third")}}
			runner := runtimeusecase.NewRunner(accounts, process, nil)
			if tc.withQuota {
				runner.SetQuotaPreflight(quota)
			}
			launcher := runSessionLauncher{
				runner: runner,
				profiles: &failedFallbackProfiles04380{multiProfileRecoverySource04360: &multiProfileRecoverySource04360{
					fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{},
				}},
				options: runtimeusecase.RuntimeLaunchOptions{SkipQuotaPreflight: tc.skipQuota},
			}
			waits := 0
			launcher.recoveryWait = func(context.Context) bool { waits++; return tc.allowWait && waits == 1 }
			forgets := 0
			err := launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
				ID: session, Path: rollout, CodexHome: home, AccountID: "a", UpstreamAccountID: "a",
				ModelProvider: "openai", LastModel: "gpt-6.1-sol", LastReasoningEffort: "ultra",
			}, []string{"exec", "resume", session, "original prompt"}, false,
				func(context.Context, string) error { forgets++; return nil })
			if len(process.calls) != tc.wantCalls || waits != 1 || forgets != tc.wantCalls-1 ||
				(err == nil) != (tc.wantCalls == 4) {
				t.Fatalf("calls=%d want=%d waits=%d forgets=%d err=%v", len(process.calls), tc.wantCalls, waits, forgets, err)
			}
			if tc.wantCalls == 4 && process.homes[3] != accounts.homes[tc.wantLast] {
				t.Fatalf("tagged recovery candidate precedence: got homes=%v, want last %s", process.homes, tc.wantLast)
			}
			for _, args := range process.calls[1:] {
				if slices.Contains(args, "original prompt") || strings.Contains(strings.Join(args, " "), "original prompt") {
					t.Fatalf("session recovery replayed original prompt: %v", args)
				}
			}
		})
	}
}

type failedFallbackOwnerFixture04380 struct {
	*failedFallbackProfiles04380
	disabled     bool
	unauthorized bool
}

func (f *failedFallbackOwnerFixture04380) SessionProfiles(ctx context.Context) ([]sessionmodel.ProfileHome, error) {
	homes, err := f.failedFallbackProfiles04380.SessionProfiles(ctx)
	if err == nil && f.disabled {
		homes[0].Enabled = false
	}
	return homes, err
}
func (f *failedFallbackOwnerFixture04380) ResolveLaunch(ctx context.Context, name string) (profilemodel.LaunchTarget, error) {
	target, err := f.failedFallbackProfiles04380.ResolveLaunch(ctx, name)
	if err == nil && name == "first" && f.unauthorized {
		target.Auth = "api-key"
	}
	return target, err
}

func TestProdex04380FailedProfileFallbackRequiresExactOwnerAndFreshEligibility(t *testing.T) {
	accounts := &runPolicyAccounts{values: []accountentity.Account{
		{ID: "a", Name: "first", Enabled: true},
		{ID: "b", Name: "second", Enabled: true},
	}, homes: map[string]string{"a": "/fixture/a", "b": "/fixture/b"}}
	quota := &failedFallbackQuota04380{ready: map[string]bool{"a": true, "b": false}}
	runner := runtimeusecase.NewRunner(accounts, nil, nil)
	runner.SetQuotaPreflight(quota)
	source := &failedFallbackOwnerFixture04380{failedFallbackProfiles04380: &failedFallbackProfiles04380{
		multiProfileRecoverySource04360: &multiProfileRecoverySource04360{fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{}},
	}}
	launcher := runSessionLauncher{runner: runner, profiles: source}
	report := sessionmodel.Report{AccountID: "a", UpstreamAccountID: "a", LastModel: "gpt-6-luna"}
	if candidate, ok := launcher.recoveryFailedProfileAfterRound04380(t.Context(), report, map[string]bool{}); !ok || candidate.AccountID != "a" {
		t.Fatalf("current authenticated-ready original was rejected: result=%+v available=%t", candidate, ok)
	}
	cases := []struct {
		name                   string
		report                 sessionmodel.Report
		attempted              map[string]bool
		disabled, unauthorized bool
		ready                  bool
	}{
		{name: "already attempted this round", report: report, attempted: map[string]bool{"a": true}, ready: true},
		{name: "different upstream owner", report: sessionmodel.Report{AccountID: "a", UpstreamAccountID: "b"}, ready: true},
		{name: "unknown original owner", report: sessionmodel.Report{AccountID: "missing", UpstreamAccountID: "missing"}, ready: true},
		{name: "disabled managed original", report: report, disabled: true, ready: true},
		{name: "original not authenticated", report: report, unauthorized: true, ready: true},
		{name: "current quota exhausted", report: report, ready: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source.disabled, source.unauthorized = tc.disabled, tc.unauthorized
			quota.ready["a"] = tc.ready
			if candidate, ok := launcher.recoveryFailedProfileAfterRound04380(t.Context(), tc.report, tc.attempted); ok {
				t.Fatalf("unqualified failed-profile fallback selected %+v", candidate)
			}
		})
	}
	source.disabled, source.unauthorized = false, false
	quota.ready["a"] = true
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, ok := launcher.recoveryFailedProfileAfterRound04380(ctx, report, nil); ok {
		t.Fatal("cancelled recovery selected an owner")
	}
}
