package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	ProviderLaunchPool(context.Context, string, string, bool) ([]profilemodel.LaunchTarget, error)
	ResolveProviderLaunch(context.Context, string, string) (profilemodel.LaunchTarget, bool, error)
	AcquireLaunchPool(context.Context, []string) (func() error, error)
	OpenAICompatibleBaseURL(context.Context, string) (string, bool, error)
}

func Run(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, arguments []string, out io.Writer) error {
	if RunHelpRequested(arguments) {
		return PrintRunHelp(out)
	}
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if selection.CLI == "agy" {
		if selection.DryRun {
			return runAntigravityDryRun(runner, out)
		}
		return runner.RunAntigravity(ctx, selection.Model, codexArguments)
	}
	if selection.DryRun {
		return runDryRun(ctx, runner, nil, selection, codexArguments, out, "")
	}
	runner.SetAutoRedeem(selection.AutoRedeem)
	launchOptions := runRuntimeLaunchOptions(selection)
	if selection.Profile != "" {
		return errors.New("--profile requires profile-aware runtime dispatch")
	}
	if selection.URL != "" {
		return runner.RunLocalProvider(ctx, selection.Account, localProviderConfig(selection), codexArguments)
	}
	if selection.Provider != "" {
		return runProfilelessProviderSelection(ctx, runner, selection, codexArguments, launchOptions)
	}
	return runParsedWithOptions(ctx, runner, sessions, selection.Account, codexArguments, launchOptions)
}

func RunProfiles(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, profiles launchProfiles, arguments []string, out io.Writer) error {
	if RunHelpRequested(arguments) {
		return PrintRunHelp(out)
	}
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if selection.CLI == "agy" {
		if selection.DryRun {
			return runAntigravityDryRun(runner, out)
		}
		return runner.RunAntigravity(ctx, selection.Model, codexArguments)
	}
	if selection.DryRun {
		return runDryRun(ctx, runner, profiles, selection, codexArguments, out, "")
	}
	runner.SetAutoRedeem(selection.AutoRedeem)
	return runProfileSelection(ctx, runner, sessions, profiles, selection, codexArguments, runRuntimeLaunchOptions(selection))
}

func RunHome(ctx context.Context, runner *runtimeusecase.Runner, sessions *sessionusecase.Catalog, home string, arguments []string, out io.Writer) error {
	if RunHelpRequested(arguments) {
		return PrintRunHelp(out)
	}
	selection, codexArguments, err := parseRunArguments(arguments)
	if err != nil {
		return err
	}
	if selection.CLI == "agy" {
		if selection.DryRun {
			return runAntigravityDryRun(runner, out)
		}
		return runner.RunAntigravity(ctx, selection.Model, codexArguments)
	}
	if selection.Profile != "" {
		return errors.New("--profile cannot override an already resolved profile home")
	}
	if selection.DryRun {
		return runDryRun(ctx, runner, nil, selection, codexArguments, out, home)
	}
	runner.SetAutoRedeem(selection.AutoRedeem)
	launchOptions := runRuntimeLaunchOptions(selection)
	if selection.URL != "" {
		if selection.Account != "" {
			return runner.RunLocalProvider(ctx, selection.Account, localProviderConfig(selection), codexArguments)
		}
		return runner.RunLocalProviderHome(ctx, home, localProviderConfig(selection), codexArguments)
	}
	if selection.Account != "" {
		return runParsedWithOptions(ctx, runner, sessions, selection.Account, codexArguments, launchOptions)
	}
	return runner.RunHome(ctx, home, codexArguments)
}

func runProfileSelection(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	codexArguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	if index, args := sessionArgument(codexArguments); index >= 0 {
		return runSessionArgumentWithLauncher(
			ctx, sessions, selection.Account, args, index,
			runSessionLauncher{runner: runner, profiles: profiles, selection: selection, options: launchOptions},
		)
	}
	if selection.URL != "" {
		return runLocalProviderSelection(ctx, runner, profiles, selection, codexArguments)
	}
	if selection.Provider != "" {
		return runProviderSelection(ctx, runner, profiles, selection, codexArguments, launchOptions)
	}
	if selection.Profile != "" {
		if profiles == nil {
			return errors.New("profile support is not configured")
		}
		target, err := profiles.ResolveLaunch(ctx, selection.Profile)
		if err != nil {
			return err
		}
		return runLaunchTarget(ctx, runner, sessions, profiles, target, codexArguments, false, launchOptions)
	}
	if selection.Account != "" {
		return runParsedWithOptionsWithRecovery(ctx, runner, sessions, profiles, selection.Account, codexArguments, launchOptions)
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return err
		}
		if active {
			return runLaunchTarget(ctx, runner, sessions, profiles, target, codexArguments, true, launchOptions)
		}
	}
	return runParsedWithOptions(ctx, runner, sessions, "", codexArguments, launchOptions)
}

func runLaunchTarget(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	profiles launchProfiles,
	target profilemodel.LaunchTarget,
	arguments []string,
	allowRotate bool,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) (runErr error) {
	var err error
	launchOptions, err = qualifiedOpenAINativeLaunch04360(ctx, profiles, target, launchOptions)
	if err != nil {
		return err
	}
	if target.AccountID != "" && target.Provider == "openai" && target.Auth != "api-key" {
		return runParsedWithOptionsWithRecovery(ctx, runner, sessions, profiles, target.AccountID, arguments, launchOptions)
	}
	if profiles == nil || target.Name == "" {
		return errors.New("profile launch metadata is incomplete")
	}
	provider, err := launchRuntimeProvider(target)
	if err != nil {
		return err
	}
	if target.Provider == "openai" {
		if acquirer, ok := profiles.(interface {
			AcquireOpenAILaunch(context.Context, string) (func() error, string, bool, string, error)
		}); ok {
			release, baseURL, compatible, authLabel, err := acquirer.AcquireOpenAILaunch(ctx, target.Name)
			if err != nil {
				return err
			}
			defer func() { runErr = errors.Join(runErr, release()) }()
			effectiveAuth := target.Auth
			if authLabel != "" {
				effectiveAuth = authLabel
			}
			if target.Auth == "api-key" && effectiveAuth != "api-key" {
				return errors.New("profile authentication changed while launch was starting")
			}
			if compatible {
				return runner.RunOpenAICompatibleProfileWithOptions(ctx, target.CodexHome, baseURL, arguments, launchOptions)
			}
			if effectiveAuth == "api-key" {
				return runner.RunDirectProfileWithOptions(ctx, target.CodexHome, arguments, launchOptions)
			}
			return runStandaloneProfileWithOptions(ctx, runner, target.CodexHome, provider, arguments, launchOptions)
		}
	}
	if provider.Kind == "copilot" || provider.Kind == "anthropic" || provider.Kind == "kiro" {
		if launchOptions.AllowAutoRotate != nil && !*launchOptions.AllowAutoRotate {
			allowRotate = false
		}
		pool, err := profiles.ProviderLaunchPool(ctx, target.Name, target.Provider, allowRotate)
		if err != nil {
			return err
		}
		return runProviderPool(ctx, runner, providerPoolRequest{
			profiles: profiles, selected: target, provider: provider, pool: pool, arguments: arguments,
			launchOptions: launchOptions,
		})
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	if target.Provider == "openai" {
		baseURL, compatible, err := profiles.OpenAICompatibleBaseURL(ctx, target.Name)
		if err != nil {
			return err
		}
		if compatible {
			return runner.RunOpenAICompatibleProfileWithOptions(ctx, target.CodexHome, baseURL, arguments, launchOptions)
		}
		if target.Auth == "api-key" {
			return runner.RunDirectProfileWithOptions(ctx, target.CodexHome, arguments, launchOptions)
		}
	}
	return runStandaloneProfileWithOptions(ctx, runner, target.CodexHome, provider, arguments, launchOptions)
}

type providerPoolRequest struct {
	profiles       launchProfiles
	selected       profilemodel.LaunchTarget
	provider       proxymodel.Provider
	pool           []profilemodel.LaunchTarget
	arguments      []string
	apiURLOverride string
	launchOptions  runtimeusecase.RuntimeLaunchOptions
}

func runProviderPool(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	request providerPoolRequest,
) (runErr error) {
	names := make([]string, 0, len(request.pool))
	runtimeProfiles := make([]proxymodel.ProviderProfile, 0, len(request.pool))
	for _, target := range request.pool {
		currentProvider, err := launchRuntimeProvider(target)
		if err != nil {
			return err
		}
		if currentProvider.Kind != request.provider.Kind {
			continue
		}
		if request.apiURLOverride != "" {
			currentProvider.APIURL = request.apiURLOverride
		}
		names = append(names, target.Name)
		runtimeProfiles = append(runtimeProfiles, proxymodel.ProviderProfile{
			Name: target.Name, Home: target.CodexHome, Provider: currentProvider, Enabled: true,
		})
	}
	if len(runtimeProfiles) == 0 {
		return errors.New("runtime provider pool is empty")
	}
	release, err := request.profiles.AcquireLaunchPool(ctx, names)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	return runner.RunProviderProfilesWithOptions(
		ctx, request.selected.CodexHome, request.provider, runtimeProfiles, request.arguments, request.launchOptions,
	)
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
	case "anthropic":
		return runtimeusecase.AnthropicProvider(
			target.Name,
			optionalProviderValue(target.ProviderConfig.APIURL),
		), nil
	case "kiro":
		return runtimeusecase.KiroProvider(target.Name), nil
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
	return runStandaloneProfileWithOptions(ctx, runner, home, provider, arguments, runtimeusecase.RuntimeLaunchOptions{})
}

func runStandaloneProfileWithOptions(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	home string,
	provider proxymodel.Provider,
	arguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	runModel := func(args []string) error {
		if provider.Kind == "" {
			return runner.RunProfileWithOptions(ctx, home, args, launchOptions)
		}
		return runner.RunProviderProfileWithOptions(ctx, home, provider, args, launchOptions)
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
	return runParsedWithOptions(ctx, runner, sessions, selector, codexArguments, runtimeusecase.RuntimeLaunchOptions{})
}

func runParsedWithOptions(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	sessions *sessionusecase.Catalog,
	selector string,
	codexArguments []string,
	launchOptions runtimeusecase.RuntimeLaunchOptions,
) error {
	if index, args := sessionArgument(codexArguments); index >= 0 {
		return runSessionArgumentWithLauncher(
			ctx, sessions, selector, args, index,
			runSessionLauncher{runner: runner, options: launchOptions},
		)
	}
	if index := nativeCommandIndex(codexArguments); index >= 0 {
		if handled, err := runNativeCommand(ctx, runner, selector, codexArguments, index); handled {
			return err
		}
	}
	return runner.RunWithOptions(ctx, selector, codexArguments, launchOptions)
}

func runSessionArgument(ctx context.Context, sessions *sessionusecase.Catalog, selector string, args []string, index int) error {
	if sessions == nil {
		return errors.New("session support is not configured")
	}
	return runSessionArgumentWithLauncher(ctx, sessions, selector, args, index, nil)
}

func runSessionArgumentWithLauncher(
	ctx context.Context,
	sessions *sessionusecase.Catalog,
	selector string,
	args []string,
	index int,
	launcher sessionusecase.Launcher,
) error {
	if sessions == nil {
		return errors.New("session support is not configured")
	}
	command := nativeCommandIndex(args)
	local := command >= 0 && localSessionCommand(args[command])
	prefix, sessionSelector := splitThreadSelector(args[index])
	launch := sessionmodel.Launch{
		AccountSelector: selector,
		SessionSelector: sessionSelector,
		IDIndex:         index,
		IDPrefix:        prefix,
		Arguments:       args,
		Local:           local,
		Delete:          command >= 0 && args[command] == "delete",
	}
	if launcher != nil {
		return sessions.ResumeArgumentsWithLauncher(ctx, launch, launcher)
	}
	return sessions.ResumeArguments(ctx, launch)
}
