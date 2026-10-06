package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	codexgateway "github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/helper/redact"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"github.com/pelletier/go-toml/v2"
)

const runDryRunConfigMaxBytes = 1 << 20

type currentLaunchProfiles interface {
	CurrentLaunch(context.Context) (profilemodel.LaunchTarget, error)
}

func runDryRun(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	codexArguments []string,
	out io.Writer,
	fixedHome string,
) error {
	if runner == nil {
		return errors.New("runtime support is not configured")
	}
	target, configured, err := resolveRunDryTarget(ctx, runner, profiles, selection, fixedHome)
	if err != nil {
		return err
	}

	projected := append([]string(nil), codexArguments...)
	runtimeProxy := true
	providerKind := ""
	if selection.URL != "" {
		projected, err = runtimeusecase.PreviewLocalProviderArguments(localProviderConfig(selection), projected)
		runtimeProxy = false
		providerKind = "local"
	} else if target.Name != "" && target.AccountID == "" && target.Provider == "openai" && profiles != nil {
		baseURL, compatible, lookupErr := profiles.OpenAICompatibleBaseURL(ctx, target.Name)
		if lookupErr != nil {
			return lookupErr
		}
		if compatible {
			projected, err = runtimeusecase.PreviewOpenAICompatibleArguments(baseURL, projected)
			runtimeProxy = false
			providerKind = "openai-compatible"
		}
	}
	if err != nil {
		return err
	}
	if runtimeProxy {
		projected, err = codexgateway.PreviewRuntimeProxyArguments(providerKind, projected)
		if err != nil {
			return err
		}
	}

	config, err := readRunDryConfig(target.CodexHome)
	if err != nil {
		return err
	}
	provider := runDryEffectiveConfig(projected, config, "model_provider")
	if provider == "" {
		provider = "openai"
	}
	model := runDryCLIModel(projected)
	if model == "" {
		model = runDryEffectiveConfig(projected, config, "model")
	}
	if model == "" {
		model = "(codex default)"
	}

	profileLabel := "(active/default)"
	if configured {
		profileLabel = "<configured>"
	}
	proxyLabel := "disabled"
	if runtimeProxy {
		proxyLabel = "would be enabled with mount /backend-api/godex"
	}

	fmt.Fprintln(out, "Godex dry run: launch diagnostics")
	fmt.Fprintln(out, "Flow: run")
	fmt.Fprintf(out, "Binary: %s\n", redact.Secrets(runner.DryRunBinaryLabel()))
	fmt.Fprintf(out, "Provider: %s\n", redact.Secrets(provider))
	fmt.Fprintf(out, "Model: %s\n", redact.Secrets(model))
	fmt.Fprintln(out, "CODEX_HOME: <CODEX_HOME>")
	fmt.Fprintf(out, "Runtime proxy: %s\n", proxyLabel)
	fmt.Fprintln(out, "Args:")
	redactedArgs := runDryRedactedArguments(projected, target.CodexHome)
	if len(redactedArgs) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, argument := range redactedArgs {
			fmt.Fprintf(out, "  %s\n", argument)
		}
	}
	fmt.Fprintln(out, "Env:")
	fmt.Fprintln(out, "  CODEX_HOME=<CODEX_HOME>")
	fmt.Fprintf(out, "Profile: %s\n", profileLabel)
	fmt.Fprintln(out, "Presidio redaction: disabled (not started)")
	fmt.Fprintln(out, "Codex/TUI not started because --dry-run was set.")
	return nil
}

func resolveRunDryTarget(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	selection runtimemodel.Selection,
	fixedHome string,
) (profilemodel.LaunchTarget, bool, error) {
	if strings.TrimSpace(fixedHome) != "" {
		return profilemodel.LaunchTarget{CodexHome: filepath.Clean(fixedHome), Provider: "openai"}, selection.Profile != "", nil
	}
	if selection.Profile != "" && profiles == nil {
		return profilemodel.LaunchTarget{}, false, errors.New("--profile requires profile-aware runtime dispatch")
	}
	if profiles != nil && selection.Provider != "" {
		target, found, err := profiles.ResolveProviderLaunch(ctx, selection.Provider, selection.Profile)
		if err != nil {
			return profilemodel.LaunchTarget{}, false, err
		}
		if found {
			return target, selection.Profile != "", nil
		}
	}
	if profiles != nil && selection.Profile != "" {
		target, err := profiles.ResolveLaunch(ctx, selection.Profile)
		return target, true, err
	}
	if profiles != nil && selection.Account != "" {
		target, err := profiles.ResolveLaunch(ctx, selection.Account)
		return target, true, err
	}
	if current, ok := profiles.(currentLaunchProfiles); ok {
		target, err := current.CurrentLaunch(ctx)
		if err != nil {
			return profilemodel.LaunchTarget{}, false, fmt.Errorf(
				"no active profile selected and no Codex-compatible profiles are available; use godex use --profile <name> or pass --profile: %w",
				err,
			)
		}
		return target, false, nil
	}
	if profiles != nil {
		target, active, err := profiles.ActiveLaunch(ctx)
		if err != nil {
			return profilemodel.LaunchTarget{}, false, err
		}
		if active {
			return target, false, nil
		}
	}
	if home := strings.TrimSpace(runner.CurrentCodexHome()); home != "" && profiles == nil {
		return profilemodel.LaunchTarget{CodexHome: filepath.Clean(home), Provider: "openai"}, false, nil
	}
	return profilemodel.LaunchTarget{}, false, errors.New(
		"no active profile selected and no Codex-compatible profiles are available; use godex use --profile <name> or pass --profile",
	)
}

func readRunDryConfig(home string) (map[string]any, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return map[string]any{}, nil
	}
	path := filepath.Join(filepath.Clean(home), "config.toml")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect dry-run Codex config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > runDryRunConfigMaxBytes {
		return nil, errors.New("dry-run Codex config.toml must be a bounded regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > runDryRunConfigMaxBytes {
		return nil, errors.New("read dry-run Codex config.toml")
	}
	var config map[string]any
	if err := toml.Unmarshal(content, &config); err != nil {
		return nil, errors.New("parse dry-run Codex config.toml")
	}
	return config, nil
}

func runDryEffectiveConfig(arguments []string, config map[string]any, key string) string {
	if value, found := runDryConfigOverride(arguments, key); found {
		return value
	}
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func runDryConfigOverride(arguments []string, key string) (string, bool) {
	value := ""
	found := false
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		assignment := ""
		consumed := 1
		switch {
		case argument == "-c" || argument == "--config":
			if index+1 < len(arguments) {
				assignment = arguments[index+1]
				consumed = 2
			}
		case strings.HasPrefix(argument, "--config="):
			assignment = strings.TrimPrefix(argument, "--config=")
		case strings.HasPrefix(argument, "-c="):
			assignment = strings.TrimPrefix(argument, "-c=")
		case strings.HasPrefix(argument, "-c") && argument != "-C":
			assignment = strings.TrimPrefix(argument, "-c")
		}
		if assignment != "" {
			var parsed map[string]any
			if toml.Unmarshal([]byte(assignment), &parsed) == nil {
				if scalar, ok := parsed[key].(string); ok {
					value, found = strings.TrimSpace(scalar), true
				}
			}
		}
		index += consumed
	}
	return value, found
}

func runDryCLIModel(arguments []string) string {
	model := ""
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		switch {
		case (argument == "--model" || argument == "-m") && index+1 < len(arguments):
			model = strings.TrimSpace(arguments[index+1])
			index++
		case strings.HasPrefix(argument, "--model="), strings.HasPrefix(argument, "-m="):
			_, value, _ := strings.Cut(argument, "=")
			model = strings.TrimSpace(value)
		}
	}
	return model
}

func runDryRedactedArguments(arguments []string, home string) []string {
	cwd, _ := os.Getwd()
	paths := []string{strings.TrimSpace(home), strings.TrimSpace(cwd)}
	result := make([]string, 0, len(arguments))
	redactNext := false
	for _, argument := range arguments {
		if redactNext {
			result = append(result, "<redacted>")
			redactNext = false
			continue
		}
		lower := strings.ToLower(argument)
		if runDrySecretFlag(lower) {
			if key, _, ok := strings.Cut(argument, "="); ok {
				argument = key + "=<redacted>"
			} else {
				redactNext = true
			}
		}
		argument = redact.Secrets(argument)
		for _, path := range paths {
			if path != "" {
				argument = strings.ReplaceAll(argument, path, "<redacted-path>")
			}
		}
		result = append(result, argument)
	}
	return result
}

func runDrySecretFlag(argument string) bool {
	key := argument
	if before, _, ok := strings.Cut(argument, "="); ok {
		key = before
	}
	key = strings.TrimLeft(key, "-")
	key = strings.ReplaceAll(key, "_", "-")
	for _, marker := range []string{"api-key", "token", "secret", "password", "authorization", "header", "cookie"} {
		if key == marker || strings.HasSuffix(key, "-"+marker) {
			return true
		}
	}
	return false
}
