package runtime

import (
	"context"
	"io"
	"slices"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type lastSettingsReader04390 struct{ shared, cwd string }

func (reader lastSettingsReader04390) List(_ context.Context, home string) ([]sessionentity.Session, error) {
	if home != reader.shared {
		return nil, nil
	}
	return []sessionentity.Session{
		{ID: legacySettingsSession04390, CWD: reader.cwd, Source: "cli", UpdatedUnix: 1},
		{ID: nativeLastExec04390, CWD: reader.cwd, Source: "exec", UpdatedUnix: 2,
			ModelProvider: "openai", LastModel: "saved-exec-model", LastReasoningEffort: "high"},
		{ID: "archived", CWD: reader.cwd, Source: "cli", UpdatedUnix: 3, Path: "/shared/archived_sessions/rollout.jsonl"},
		{ID: "child", CWD: reader.cwd, Source: "exec", UpdatedUnix: 4, ParentThreadID: nativeLastExec04390},
	}, nil
}

func TestProdex04390SuperLastRestoresSettingsWithoutSelectingThread(t *testing.T) {
	home, shared := t.TempDir(), t.TempDir()
	t.Chdir(home)
	catalog := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, lastSettingsReader04390{shared, home}, nil)
	catalog.SetSharedCodexHome(shared)
	for _, local := range []bool{false, true} {
		args := []string{"--profile", "main"}
		if local {
			args = append(args, "--url", "http://127.0.0.1:11434", "--model", "explicit-local")
		}
		options, err := parseSuperArguments(append(args, "exec", "resume", "--last", "continue"))
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := resolveSuperResumeOptions(t.Context(), catalog, options)
		if err != nil || !nativeResumeLastRequested(resolved.codexArgs) || slices.Contains(resolved.codexArgs, nativeLastExec04390) {
			t.Fatalf("Super resolved a native last thread: %q error=%v", resolved.codexArgs, err)
		}
		model, _ := runDryConfigOverride(resolved.codexArgs, "model")
		if !local && model != "saved-exec-model" {
			t.Fatalf("Super lost shared model: %q", model)
		}
		if effort, _ := runDryConfigOverride(resolved.codexArgs, "model_reasoning_effort"); effort != "high" {
			t.Fatalf("Super lost shared effort: %q", effort)
		}
	}
}

type nativeLastAccounts04390 struct{ sharedNativeSessionAccounts }

func (accounts nativeLastAccounts04390) Current(ctx context.Context) (accountentity.Account, error) {
	all, err := accounts.List(ctx)
	return all[0], err
}

func (accounts nativeLastAccounts04390) Resolve(ctx context.Context, _ string) (accountentity.Account, error) {
	return accounts.Current(ctx)
}

func TestProdex04390ManagedLastKeepsActiveHomeAndNativeArguments(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	accounts := nativeLastAccounts04390{}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	catalog := sessionusecase.NewCatalog(accounts, sharedNativeSessionReader{project: project}, runner)
	args := []string{"exec", "resume", "--last", "continue"}
	if err := Run(t.Context(), runner, catalog, args, io.Discard); err != nil {
		t.Fatal(err)
	}
	if process.home != "/profiles/one" || !slices.Equal(process.arguments, args) {
		t.Fatalf("managed last replaced native selection: home=%q args=%q", process.home, process.arguments)
	}
}

func TestProdex04390NativeLastActiveProfileKeepsOtherDurableOwnersEligible(t *testing.T) {
	for _, active := range []bool{true, false} {
		accounts := nativeLastAccounts04390{}
		var captured proxymodel.Config
		runner := runtimeusecase.NewRunner(accounts, &kiroShortcutProcess{}, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
			captured = config
			return &kiroShortcutProxy{}, nil
		})
		target := profilemodel.LaunchTarget{AccountID: "one", Provider: "openai", Auth: "chatgpt"}
		err := runLaunchTarget(t.Context(), runner, nil, nil, target,
			[]string{"exec", "resume", "--last", "continue"}, active,
			runtimeusecase.RuntimeLaunchOptions{SkipQuotaPreflight: true, SmartContextEnabled: true})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := captured.Accounts(t.Context())
		want := 1
		if active {
			want = 2
		}
		if err != nil || len(pool) != want || captured.PreferredAccount != "one" || !captured.SmartContextEnabled {
			t.Fatalf("native last lost active pool or options: pool=%d error=%v", len(pool), err)
		}
	}
}

const nativeLastExec04390 = "01900000-0000-7000-8000-000000000167"

func TestProdex04390NativeLastKeepsSelectionWithCodex(t *testing.T) {
	home, shared := t.TempDir(), t.TempDir()
	t.Chdir(home)
	profiles := &superResumeProfiles04390{fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "main", CodexHome: home, Provider: "openai", Auth: "api-key",
	}}}
	catalog := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, lastSettingsReader04390{shared, home}, nil)
	catalog.SetProfiles(profiles)
	catalog.SetSharedCodexHome(shared)
	for _, test := range []struct {
		name    string
		catalog *sessionusecase.Catalog
		args    []string
		want    []string
	}{
		{"shared settings", catalog, []string{"exec", "resume", "--last", "continue"},
			[]string{"-c", `model_reasoning_effort="high"`, "-c", `model="saved-exec-model"`, "exec", "resume", "--last", "continue"}},
		{"native empty catalog", nil, []string{"exec", "resume", "--last", "--all", "continue"},
			[]string{"exec", "resume", "--last", "--all", "continue"}},
		{"explicit settings", catalog, []string{"exec", "resume", "-c", `model="explicit"`, "-c", `model_reasoning_effort="low"`, "--last", "continue"},
			[]string{"exec", "resume", "-c", `model="explicit"`, "-c", `model_reasoning_effort="low"`, "--last", "continue"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := &fakeRunnerProcess{}
			runner := runtimeusecase.NewRunner(nil, process, nil)
			args := append([]string{"--profile", "main", "--skip-quota-check", "--no-auto-rotate"}, test.args...)
			if err := RunProfiles(t.Context(), runner, test.catalog, profiles, args, io.Discard); err != nil {
				t.Fatal(err)
			}
			if process.home != home || !slices.Equal(process.arguments, test.want) {
				t.Fatalf("native last launch home=%q args=%q, want %q", process.home, process.arguments, test.want)
			}
		})
	}
}
