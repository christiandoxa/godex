package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type exactSessionContinuationProcess04377 struct {
	path    string
	calls   [][]string
	effects int
}

func (process *exactSessionContinuationProcess04377) Run(_ context.Context, _ string, args []string) error {
	process.calls = append(process.calls, append([]string(nil), args...))
	if len(process.calls) != 1 {
		if slices.Contains(args, "original prompt") {
			process.effects++
		}
		return nil
	}
	file, err := os.OpenFile(process.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteString(
		`{"type":"response_item","payload":{"type":"message","role":"user"}}` + "\n" +
			`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"write-once","output":"completed"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"Selected model is at capacity.","codex_error_info":"server_overloaded"}}}` + "\n",
	)
	if closeErr := file.Close(); err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	process.effects++
	return errors.New("native capacity failure")
}

func TestProdex04377ExactSessionToolEffectContinuesOnceWithoutReplay(t *testing.T) {
	const id = recoverySession04377
	home := t.TempDir()
	path := filepath.Join(home, "rollout-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	process := &exactSessionContinuationProcess04377{path: path}
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
	err := launcher.RunSessionReportWithRecovery(
		t.Context(),
		sessionmodel.Report{
			ID: id, Path: path, CodexHome: home, AccountID: "a", UpstreamAccountID: "a",
			ModelProvider: "openai", LastModel: "gpt-6.1-sol", LastReasoningEffort: "ultra",
		},
		[]string{"exec", "resume", id, "original prompt"}, false,
		func(_ context.Context, got string) error {
			if got != id || len(process.calls) != 1 {
				t.Fatalf("binding release id/calls=%q/%d", got, len(process.calls))
			}
			released++
			return nil
		},
	)
	if err != nil || len(process.calls) != 2 || released != 1 || process.effects != 1 {
		t.Fatalf("continuation err=%v calls=%d releases=%d effects=%d", err, len(process.calls), released, process.effects)
	}
	resumed := process.calls[1]
	for _, want := range []string{"exec", "resume", id, recoveryContinuationPrompt04360,
		`model="gpt-6.1-sol"`, `model_reasoning_effort="ultra"`} {
		if !slices.Contains(resumed, want) {
			t.Fatalf("continuation missing %q: %#v", want, resumed)
		}
	}
	if strings.Contains(strings.Join(resumed, " "), "original prompt") {
		t.Fatalf("original prompt was replayed: %#v", resumed)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(content), `"call_id":"write-once"`); got != 1 {
		t.Fatalf("completed tool effect count=%d want 1", got)
	}
}
