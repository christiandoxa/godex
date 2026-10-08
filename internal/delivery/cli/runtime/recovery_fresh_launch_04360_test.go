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

type creatingSessionProcess04360 struct {
	home                     string
	calls                    [][]string
	extra, marker, cancelled bool
	variant                  string
}

func (process *creatingSessionProcess04360) Run(_ context.Context, _ string, args []string) error {
	process.calls = append(process.calls, append([]string(nil), args...))
	if len(process.calls) > 1 {
		return nil
	}
	sessionDir := filepath.Join(process.home, "sessions", "2026", "10", "08")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return err
	}
	newSession := func(id string) error {
		body := `{"type":"session_meta","payload":{"id":"` + id + `","model_provider":"openai","source":"exec"}}` + "\n" +
			`{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"ultra"}}` + "\n"
		if process.marker {
			variant := `{"type":"error","error":{"code":"usage_limit_reached"}}`
			if process.variant != "" {
				variant = `{"type":"event_msg","payload":{"type":"error","codex_error_info":"` + process.variant + `"}}`
			}
			body += `{"type":"response_item","payload":{"role":"user"}}` + "\n" + variant + "\n"
		}
		return os.WriteFile(filepath.Join(sessionDir, "rollout-2026-10-08T10-00-00-"+id+".jsonl"), []byte(body), 0600)
	}
	if err := newSession(newSessionID04360); err != nil {
		return err
	}
	if process.extra {
		if err := newSession(otherSessionID04360); err != nil {
			return err
		}
	}
	if process.cancelled {
		return recoveryCancelled04360{}
	}
	return errors.New("synthetic usage limit")
}

func TestProdex04360RunProfilesRecoversExactlyOneNewExecSession(t *testing.T) {
	for _, tc := range []struct {
		name                                           string
		marker, extra, backup, cancelled, noAutoRotate bool
		wantCalls, wantForget                          int
	}{
		{"fresh accepted limit", true, false, true, false, false, 2, 1},
		{"structured rate limit", true, false, true, false, false, 2, 1},
		{"structured transport failure", true, false, true, false, false, 2, 1},
		{"two concurrent sessions", true, true, true, false, false, 1, 0},
		{"missing post-child evidence", false, false, true, false, false, 1, 0},
		{"no eligible backup profile", true, false, false, false, false, 1, 0},
		{"cancelled process", true, false, true, true, false, 1, 0},
		{"explicit no auto rotate", true, false, true, false, true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			source := &runPolicyAccounts{values: []accountentity.Account{
				{ID: "a", Name: "primary", Enabled: true},
				{ID: "b", Name: "standby", Enabled: true},
			}, homes: map[string]string{"a": home, "b": home}}
			process := &creatingSessionProcess04360{home: home, marker: tc.marker, extra: tc.extra, cancelled: tc.cancelled}
			if tc.name == "structured rate limit" {
				process.variant = "rate_limit_exceeded"
			}
			if tc.name == "structured transport failure" {
				process.variant = "response_stream_disconnected"
			}
			runner := runtimeusecase.NewRunner(source, process, nil)
			sessions := sessionusecase.NewCatalog(source, sessionrepo.NewReader(), nil)
			sessions.SetSharedCodexHome(home)
			forgotten := 0
			sessions.SetBindingForget(func(_ context.Context, id string) error {
				if id != newSessionID04360 {
					t.Fatalf("forgot wrong session %s", id)
				}
				forgotten++
				return nil
			})
			profiles := &verifiedRecoveryProfiles04360{
				fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
					target: profilemodel.LaunchTarget{Name: "primary", AccountID: "a", CodexHome: home, Provider: "openai", Auth: "chatgpt"},
					active: true,
				}, eligible: tc.backup,
			}
			args := []string{"exec", "initial prompt"}
			if tc.noAutoRotate {
				args = append([]string{"--no-auto-rotate"}, args...)
			}
			err := RunProfiles(t.Context(), runner, sessions, profiles, args, io.Discard)
			if len(process.calls) != tc.wantCalls || forgotten != tc.wantForget {
				t.Fatalf("calls=%d forget=%d err=%v", len(process.calls), forgotten, err)
			}
			if tc.wantCalls == 2 {
				if err != nil {
					t.Fatal(err)
				}
				resumed := process.calls[1]
				for _, value := range []string{"exec", "resume", newSessionID04360,
					`model="gpt-6.1-sol"`, `model_reasoning_effort="ultra"`, recoveryContinuationPrompt04360} {
					if !slices.Contains(resumed, value) {
						t.Fatalf("lost session/model identity %q: %#v", value, resumed)
					}
				}
				if slices.Contains(resumed, "initial prompt") {
					t.Fatalf("initial prompt replayed: %#v", resumed)
				}
			} else if err == nil {
				t.Fatal("failed initial process unexpectedly returned success")
			}
		})
	}
}
