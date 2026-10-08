package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type multiGenerationFreshProcess04360 struct {
	home       string
	emitSecond bool
	calls      [][]string
	homes      []string
}

func (p *multiGenerationFreshProcess04360) Run(_ context.Context, home string, args []string) error {
	p.homes = append(p.homes, home)
	p.calls = append(p.calls, append([]string(nil), args...))
	n := len(p.calls)
	path := filepath.Join(p.home, "sessions", "2026", "10", "08",
		"rollout-2026-10-08T10-00-00-"+newSessionID04360+".jsonl")
	record := []byte(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"codex_error_info":"rate_limit_exceeded"}}` + "\n")
	if n == 1 {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		start := []byte(`{"type":"session_meta","payload":{"id":"` + newSessionID04360 + `","source":"exec","model_provider":"openai"}}` + "\n" +
			`{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"ultra"}}` + "\n")
		if err := os.WriteFile(path, append(start, record...), 0600); err != nil {
			return err
		}
		return errors.New("first rate-limit failure")
	}
	if n == 2 {
		if p.emitSecond {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, err = f.Write(record)
			if err != nil {
				_ = f.Close()
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
		}
		return errors.New("second failed child")
	}
	return nil
}

func TestProdex04360RunProfilesFreshSessionMultigenerationRequiresNewEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		secondEvidence        bool
		wantCalls, wantForget int
		success               bool
	}{
		{"first and second errors newly observed", true, 3, 2, true},
		{"second error not newly observed", false, 2, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			accounts := &runPolicyAccounts{
				values: []accountentity.Account{
					{ID: "a", Name: "first", Enabled: true},
					{ID: "b", Name: "second", Enabled: true},
					{ID: "c", Name: "third", Enabled: true},
				},
				homes: map[string]string{"a": home, "b": filepath.Join(home, "second"), "c": filepath.Join(home, "third")},
			}
			process := &multiGenerationFreshProcess04360{home: home, emitSecond: tc.secondEvidence}
			runner := runtimeusecase.NewRunner(accounts, process, nil)
			sessions := sessionusecase.NewCatalog(accounts, sessionrepo.NewReader(), nil)
			sessions.SetSharedCodexHome(home)
			forget := 0
			sessions.SetBindingForget(func(_ context.Context, id string) error {
				if id != newSessionID04360 {
					t.Fatalf("wrong owner release %q", id)
				}
				forget++
				if forget > len(process.calls) {
					t.Fatal("released binding before a child exited")
				}
				return nil
			})
			profiles := &multiProfileRecoverySource04360{
				fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
					target: profilemodel.LaunchTarget{
						Name: "first", AccountID: "a", CodexHome: home, Provider: "openai", Auth: "chatgpt",
					}, active: true,
				},
			}
			err := RunProfiles(t.Context(), runner, sessions, profiles, []string{
				"exec", "--cyber-access-program", "standard", "original user prompt",
			}, io.Discard)
			if len(process.calls) != tc.wantCalls || forget != tc.wantForget || (err == nil) != tc.success {
				t.Fatalf("calls=%d forget=%d err=%v", len(process.calls), forget, err)
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
					t.Fatalf("attempt %d used profile home %q expected %q", i, used, expected)
				}
			}
			for _, argv := range process.calls[1:] {
				for _, preserved := range []string{"resume", newSessionID04360,
					"--cyber-access-program", "standard",
					`model="gpt-6.1-sol"`, `model_reasoning_effort="ultra"`,
					recoveryContinuationPrompt04360} {
					if !slices.Contains(argv, preserved) {
						t.Fatalf("lost native recovery %q in %#v", preserved, argv)
					}
				}
				if slices.Contains(argv, "original user prompt") {
					t.Fatalf("replayed previous user prompt: %#v", argv)
				}
			}
		})
	}
}
