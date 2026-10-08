package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func TestProdex04360TransientPoolRecyclesOnlyWithFreshEvidence(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name, variant                              string
		emit                                       map[int]bool
		failUntil, wantCalls, wantForget, wantWait int
		wantSuccess                                bool
	}{
		{"transient new evidence recycles pool", "rate_limit_exceeded", map[int]bool{1: true, 2: true, 3: true}, 3, 4, 3, 1, true},
		{"usage limit does not recycle pool", "usage_limit_exceeded", map[int]bool{1: true, 2: true, 3: true}, 3, 3, 2, 0, false},
		{"no evidence after second child stops", "rate_limit_exceeded", map[int]bool{1: true, 2: false}, 3, 2, 1, 0, false},
		{"cancelled at transient wait", "rate_limit_exceeded", map[int]bool{1: true, 2: true, 3: true}, 3, 3, 2, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "rollout-"+id+".jsonl")
			if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := &sequentialRecoveryProcess04360{
				path: path, emit: tc.emit, failUntil: tc.failUntil, variant: tc.variant,
			}
			accounts := &runPolicyAccounts{
				values: []accountentity.Account{
					{ID: "a", Name: "first", Enabled: true},
					{ID: "b", Name: "second", Enabled: true},
					{ID: "c", Name: "third", Enabled: true},
				},
				homes: map[string]string{
					"a": home, "b": filepath.Join(home, "second"), "c": filepath.Join(home, "third"),
				},
			}
			runCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			waits, forgets := 0, 0
			launcher := runSessionLauncher{
				runner: runtimeusecase.NewRunner(accounts, process, nil),
				profiles: &multiProfileRecoverySource04360{
					fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{},
				},
				recoveryWait: func(context.Context) bool {
					waits++
					if tc.name == "cancelled at transient wait" {
						cancel()
						return false
					}
					return waits == 1
				},
			}
			err := launcher.RunSessionReportWithRecovery(runCtx, sessionmodel.Report{
				ID: id, Path: path, CodexHome: home, AccountID: "a", UpstreamAccountID: "a",
				ModelProvider: "openai", LastModel: "gpt-6.1-sol", LastReasoningEffort: "ultra",
			}, []string{"exec", "resume", id, "old prompt"}, false,
				func(context.Context, string) error { forgets++; return nil })
			if len(process.calls) != tc.wantCalls || forgets != tc.wantForget || waits != tc.wantWait || (err == nil) != tc.wantSuccess {
				t.Fatalf("calls=%d want=%d forgets=%d want=%d waits=%d want=%d err=%v", len(process.calls), tc.wantCalls, forgets, tc.wantForget, waits, tc.wantWait, err)
			}
			if tc.name == "cancelled at transient wait" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled recovery returned %v instead of context.Canceled", err)
			}
			if tc.wantSuccess && !reflect.DeepEqual(process.homes, []string{
				home, filepath.Join(home, "second"), filepath.Join(home, "third"), filepath.Join(home, "second"),
			}) {
				t.Fatalf("transient round did not recycle only after exhaustion: homes=%#v", process.homes)
			}
		})
	}
}

func TestProdex04360RecoveryWaitRespectsCancellation(t *testing.T) {
	if runtimeRecoveryRetryInterval04360 != 5*time.Second {
		t.Fatalf("tagged Prodex retry interval drift: %v", runtimeRecoveryRetryInterval04360)
	}

	delayed, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	started := time.Now()
	if waitRuntimeRecoveryRound04360(delayed) {
		t.Fatal("wait should stop on a fresh cancellation")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("recovery wait failed to abort substantially before five-second retry interval")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitRuntimeRecoveryRound04360(ctx) {
		t.Fatal("cancelled recovery context waited and relaunched")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error=%v", ctx.Err())
	}
}
