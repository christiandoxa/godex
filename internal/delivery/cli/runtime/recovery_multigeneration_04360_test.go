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
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type multiProfileRecoverySource04360 struct {
	*fakeLocalLaunchProfiles
}

func (source *multiProfileRecoverySource04360) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return []sessionmodel.ProfileHome{
		{Name: "first", AccountID: "a", Provider: "openai", Enabled: true},
		{Name: "second", AccountID: "b", Provider: "openai", Enabled: true},
		{Name: "third", AccountID: "c", Provider: "openai", Enabled: true},
	}, nil
}
func (source *multiProfileRecoverySource04360) ResolveLaunch(_ context.Context, name string) (profilemodel.LaunchTarget, error) {
	accounts := map[string]string{"second": "b", "third": "c"}
	id := accounts[name]
	if id == "" {
		return profilemodel.LaunchTarget{}, errors.New("not a backup profile")
	}
	return profilemodel.LaunchTarget{Name: name, AccountID: id, Provider: "openai", Auth: "chatgpt", CodexHome: "/fixture"}, nil
}

type sequentialRecoveryProcess04360 struct {
	path      string
	failUntil int
	emit      map[int]bool
	variant   string
	quotaCode string
	calls     [][]string
	homes     []string
}

func (p *sequentialRecoveryProcess04360) Run(_ context.Context, home string, args []string) error {
	p.homes = append(p.homes, home)
	p.calls = append(p.calls, append([]string(nil), args...))
	n := len(p.calls)
	if n > p.failUntil {
		return nil
	}
	if p.emit[n] {
		f, err := os.OpenFile(p.path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		variant := p.variant
		if variant == "" {
			variant = "rate_limit_exceeded"
		}
		failure := `{"type":"error","error":{"codex_error_info":"` + variant + `"}}`
		if p.quotaCode != "" {
			failure = `{"type":"error","error":{"code":"` + p.quotaCode + `"}}`
		}
		_, err = f.WriteString(`{"type":"response_item","payload":{"type":"message","role":"user"}}` + "\n" + failure + "\n")
		if err != nil {
			_ = f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	}
	return errors.New("synthetic recoverable failure")
}

func TestProdex04360KnownSessionRecoveryWalksDistinctProfilesUnderFreshEvidence(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name                  string
		failUntil             int
		emit                  map[int]bool
		quotaCode             string
		wantCalls, wantForget int
		success               bool
	}{
		{"two generations then success", 2, map[int]bool{1: true, 2: true}, "", 3, 2, true},
		{"second failure without new accepted event must stop", 2, map[int]bool{1: true, 2: false}, "", 2, 1, false},
		{"every candidate exhausted without replay", 3, map[int]bool{1: true, 2: true, 3: true}, "", 3, 2, false},
		{"provider quota code rotates to backup", 1, map[int]bool{1: true}, "insufficient_quota", 2, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "rollout-"+id+".jsonl")
			if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			process := &sequentialRecoveryProcess04360{path: path, failUntil: tc.failUntil, emit: tc.emit, quotaCode: tc.quotaCode}
			accounts := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "first", Enabled: true},
				{ID: "b", Name: "second", Enabled: true},
				{ID: "c", Name: "third", Enabled: true},
			}, homes: map[string]string{"a": home, "b": filepath.Join(home, "second"), "c": filepath.Join(home, "third")}}
			runner := runtimeusecase.NewRunner(accounts, process, nil)
			launcher := runSessionLauncher{runner: runner, profiles: &multiProfileRecoverySource04360{
				fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{},
			}, recoveryWait: func(context.Context) bool { return false }}
			forget := 0
			err := launcher.RunSessionReportWithRecovery(t.Context(), sessionmodel.Report{
				ID: id, Path: path, CodexHome: home, AccountID: "a", UpstreamAccountID: "a",
				ModelProvider: "openai", LastModel: "gpt-6.1-sol", LastReasoningEffort: "ultra",
			}, []string{"exec", "resume", id, "--cyber-access-program", "daybreak_blue", "original prompt"}, false,
				func(context.Context, string) error { forget++; return nil })
			if len(process.calls) != tc.wantCalls || forget != tc.wantForget || (err == nil) != tc.success {
				t.Fatalf("calls=%d want=%d forget=%d want=%d err=%v", len(process.calls), tc.wantCalls, forget, tc.wantForget, err)
			}
			for i, used := range process.homes {
				expected := home
				if i == 1 {
					expected = filepath.Join(home, "second")
				}
				if i == 2 {
					expected = filepath.Join(home, "third")
				}
				if used != expected {
					t.Fatalf("attempt %d selected incorrect or repeated profile: %q want %q", i, used, expected)
				}
			}
			for _, argv := range process.calls[1:] {
				if !slices.Contains(argv, "--cyber-access-program") || !slices.Contains(argv, "daybreak_blue") ||
					!slices.Contains(argv, `model="gpt-6.1-sol"`) ||
					!slices.Contains(argv, `model_reasoning_effort="ultra"`) ||
					strings.Contains(strings.Join(argv, " "), "original prompt") {
					t.Fatalf("unsafe multiple-generation plan: %#v", argv)
				}
			}
		})
	}
}
