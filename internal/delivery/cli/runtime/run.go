package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func Launch(ctx context.Context, runner *runtimeusecase.Runner) error {
	return runner.Run(ctx, "", nil)
}

type launchProfiles interface {
	ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error)
	ActiveLaunch(context.Context) (profilemodel.LaunchTarget, bool, error)
	AcquireLaunch(context.Context, string) (func() error, error)
}

func Run(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, arguments []string) error {
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if selection.Profile != "" {
		return errors.New("--profile requires profile-aware runtime dispatch")
	}
	return runParsed(ctx, runner, sessions, selection.Account, codexArguments)
}

func RunProfiles(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, profiles launchProfiles, arguments []string) error {
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	return runProfileSelection(ctx, runner, sessions, profiles, selection, codexArguments)
}

func RunHome(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, home string, arguments []string) error {
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if selection.Profile != "" {
		return errors.New("--profile cannot override an already resolved profile home")
	}
	if selection.Account != "" {
		return runParsed(ctx, runner, sessions, selection.Account, codexArguments)
	}
	return runner.RunHome(ctx, home, codexArguments)
}

func runProfileSelection(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, profiles launchProfiles, selection runtimemodel.Selection, codexArguments []string) error {
	if selection.Profile != "" {
		if profiles == nil {
			return errors.New("profile support is not configured")
		}
		target, err := profiles.ResolveLaunch(ctx, selection.Profile)
		if err != nil {
			return err
		}
		return runLaunchTarget(ctx, runner, sessions, profiles, target, codexArguments)
	}
	if selection.Account != "" {
		return runParsed(ctx, runner, sessions, selection.Account, codexArguments)
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			return runLaunchTarget(ctx, runner, sessions, profiles, target, codexArguments)
		}
	}
	return runParsed(ctx, runner, sessions, "", codexArguments)
}

func runLaunchTarget(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	target profilemodel.LaunchTarget,
	arguments []string,
) (runErr error) {
	if target.AccountID != "" {
		return runParsed(ctx, runner, sessions, target.AccountID, arguments)
	}
	if profiles == nil || target.Name == "" {
		return errors.New("profile launch metadata is incomplete")
	}
	provider, err := launchRuntimeProvider(target)
	if err != nil {
		return err
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runStandaloneProfile(ctx, runner, target.CodexHome, provider, arguments)
}

func launchRuntimeProvider(target profilemodel.LaunchTarget) (proxymodel.Provider, error) {
	switch target.Provider {
	case "", "openai":
		return proxymodel.Provider{}, nil
	case "copilot":
		return runtimeusecase.CopilotProvider(
			target.Name,
			optionalProviderValue(target.ProviderConfig.Host),
			optionalProviderValue(target.ProviderConfig.Login),
			optionalProviderValue(target.ProviderConfig.APIURL),
		), nil
	default:
		return proxymodel.Provider{}, fmt.Errorf("profile provider %q is not implemented yet", target.Provider)
	}
}

func optionalProviderValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func runStandaloneProfile(ctx context.Context, runner *runtimeusecase.Runner, home string, provider proxymodel.Provider, arguments []string) error {
	runModel := func(args []string) error {
		if provider.Kind == "" {
			return runner.RunProfile(ctx, home, args)
		}
		return runner.RunProviderProfile(ctx, home, provider, args)
	}
	if index, args := sessionArgument(arguments); index >= 0 {
		_ = index
		return runModel(args)
	}
	if index := nativeCommandIndex(arguments); index >= 0 {
		if unsafeNativeCommand(arguments, index) {
			return errors.New("native command bypasses Godex routing; use godex exec or godex app-server without daemon/proxy")
		}
		switch arguments[index] {
		case "logout":
			return errors.New("use godex logout to safely mutate managed credentials")
		case "login":
			return runStandaloneLoginStatus(ctx, runner, home, arguments, index)
		case "mcp", "features", "completion", "debug", "config", "delete", "archive", "unarchive", "version", "--version":
			return runner.RunHome(ctx, home, arguments)
		case "resume", "fork", "queue":
			return runModel(arguments)
		}
	}
	return runModel(arguments)
}

func runStandaloneLoginStatus(ctx context.Context, runner *runtimeusecase.Runner, home string, arguments []string, index int) error {
	status := nextCommandWord(arguments, index+1)
	if status < 0 || arguments[status] != "status" {
		return errors.New("use godex login to register and safely update managed credentials")
	}
	return runner.RunHome(ctx, home, arguments)
}

func runParsed(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, selector string, codexArguments []string) error {
	if index, args := sessionArgument(codexArguments); index >= 0 {
		return runSessionArgument(ctx, sessions, selector, args, index)
	}
	if index := nativeCommandIndex(codexArguments); index >= 0 {
		if handled, err := runNativeCommand(ctx, runner, selector, codexArguments, index); handled {
			return err
		}
	}
	return runner.Run(ctx, selector, codexArguments)
}

func runSessionArgument(ctx context.Context, sessions *sessionusecase.Catalog, selector string, args []string, index int) error {
	if sessions == nil {
		return errors.New("session support is not configured")
	}
	command := nativeCommandIndex(args)
	local := command >= 0 && localSessionCommand(args[command])
	prefix, sessionSelector := splitThreadSelector(args[index])
	return sessions.ResumeArguments(ctx, sessionmodel.Launch{
		AccountSelector: selector,
		SessionSelector: sessionSelector,
		IDIndex:         index,
		IDPrefix:        prefix,
		Arguments:       args,
		Local:           local,
	})
}

func localSessionCommand(command string) bool {
	return command == "delete" || command == "archive" || command == "unarchive"
}

func splitThreadSelector(value string) (string, string) {
	if selector, ok := strings.CutPrefix(value, "--thread="); ok {
		return "--thread=", selector
	}
	return "", value
}

func runNativeCommand(ctx context.Context, runner *runtimeusecase.Runner, selector string, arguments []string, index int) (bool, error) {
	if unsafeNativeCommand(arguments, index) {
		return true, errors.New("native command bypasses Godex routing; use godex exec or godex app-server without daemon/proxy")
	}
	switch arguments[index] {
	case "logout":
		return true, errors.New("use godex logout to safely mutate managed credentials")
	case "login":
		return true, runNativeLoginStatus(ctx, runner, selector, arguments, index)
	case "mcp", "features", "completion", "debug", "config", "delete", "archive", "unarchive", "version", "--version":
		return true, runner.RunLocal(ctx, selector, arguments)
	case "resume", "fork", "queue":
		return true, runner.RunCurrent(ctx, selector, arguments)
	default:
		return false, nil
	}
}

func runNativeLoginStatus(ctx context.Context, runner *runtimeusecase.Runner, selector string, arguments []string, index int) error {
	status := nextCommandWord(arguments, index+1)
	if status < 0 || arguments[status] != "status" {
		return errors.New("use godex login to register and safely update managed credentials")
	}
	return runner.RunLocal(ctx, selector, arguments)
}

func unsafeNativeCommand(args []string, command int) bool {
	nested := nextCommandWord(args, command+1)
	if nested < 0 {
		return false
	}
	switch args[command] {
	case "debug":
		return args[nested] == "app-server"
	case "app-server":
		return args[nested] == "daemon" || args[nested] == "proxy"
	}
	return false
}
