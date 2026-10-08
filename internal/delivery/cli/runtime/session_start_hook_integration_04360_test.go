package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"

	"github.com/pelletier/go-toml/v2"
)

type codexHookSessionProcess04360 struct {
	home     string
	calls    [][]string
	hookUsed bool
}

func (p *codexHookSessionProcess04360) Run(_ context.Context, _ string, args []string) error {
	p.calls = append(p.calls, append([]string(nil), args...))
	if len(p.calls) > 1 {
		return nil
	}
	dir := filepath.Join(p.home, "sessions", "2026", "10", "08")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, id := range []string{newSessionID04360, otherSessionID04360} {
		body := []byte(`{"type":"session_meta","payload":{"id":"` + id + `","model_provider":"openai","source":"exec"}}` + "\n" +
			`{"type":"turn_context","payload":{"model":"gpt-6.1-sol","effort":"ultra"}}` + "\n" +
			`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n")
		if err := os.WriteFile(filepath.Join(dir, "rollout-2026-10-08T10-00-00-"+id+".jsonl"), body, 0600); err != nil {
			return err
		}
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-c" || !strings.HasPrefix(args[i+1], "hooks.SessionStart=") {
			continue
		}
		var config struct {
			Hooks struct {
				SessionStart []struct {
					Hooks []struct {
						Command string `toml:"command"`
					} `toml:"hooks"`
				} `toml:"SessionStart"`
			} `toml:"hooks"`
		}
		if err := toml.Unmarshal([]byte(args[i+1]), &config); err != nil {
			return err
		}
		if len(config.Hooks.SessionStart) != 1 || len(config.Hooks.SessionStart[0].Hooks) != 1 {
			return errors.New("unrecognized Codex SessionStart hook shape")
		}
		command := config.Hooks.SessionStart[0].Hooks[0].Command
		separator := "' '"
		if runtime.GOOS == "windows" {
			separator = `" "`
		}
		index := strings.LastIndex(command, separator)
		if index < 0 {
			return errors.New("missing marker path in Codex hook")
		}
		marker := command[index+len(separator):]
		if len(marker) < 2 {
			return errors.New("invalid marker argument")
		}
		marker = strings.TrimSuffix(marker, separator[:1])
		if runtime.GOOS != "windows" {
			marker = strings.ReplaceAll(marker, `'"'"'`, "'")
		}
		handled, err := HandleSessionStartNotify04360(
			[]string{sessionStartNotifyCommand04360, marker,
				`{"thread-id":"` + newSessionID04360 + `"}`,
			}, nil)
		if err != nil || !handled {
			return errors.New("session marker was not delivered")
		}
		p.hookUsed = true
	}
	if !p.hookUsed {
		return errors.New("SessionStart was not injected")
	}
	return errors.New("synthetic usage limit after SessionStart hook")
}

func TestProdex04360RunProfilesNativeHookDisambiguatesConcurrentNewSessions(t *testing.T) {
	home := t.TempDir()
	source := &runPolicyAccounts{
		values: []accountentity.Account{
			{ID: "a", Name: "first", Enabled: true},
			{ID: "b", Name: "backup", Enabled: true},
		},
		homes: map[string]string{"a": home, "b": home},
	}
	process := &codexHookSessionProcess04360{home: home}
	runner := runtimeusecase.NewRunner(source, process, nil)
	sessions := sessionusecase.NewCatalog(source, sessionrepo.NewReader(), nil)
	sessions.SetSharedCodexHome(home)
	forgotten := 0
	sessions.SetBindingForget(func(_ context.Context, id string) error {
		if id != newSessionID04360 {
			t.Fatalf("wrong session was released %q", id)
		}
		forgotten++
		return nil
	})
	profiles := &verifiedRecoveryProfiles04360{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
			target: profilemodel.LaunchTarget{
				Name: "first", AccountID: "a", CodexHome: home,
				Provider: "openai", Auth: "chatgpt",
			}, active: true,
		}, eligible: true,
	}
	err := RunProfiles(t.Context(), runner, sessions, profiles, []string{"exec", "original task"}, io.Discard)
	if err != nil || !process.hookUsed || len(process.calls) != 2 || forgotten != 1 {
		t.Fatalf("verified native callback not honored: calls=%d marker=%t release=%d err=%v",
			len(process.calls), process.hookUsed, forgotten, err)
	}
}
