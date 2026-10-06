package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type gatewayOptions struct {
	listen       string
	provider     string
	baseURL      string
	apiKey       string
	smartContext bool
	presidio     bool
	noPresidio   bool
}

func Gateway(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	out io.Writer,
	arguments []string,
) error {
	options, err := parseGatewayArguments(arguments)
	if err != nil {
		return err
	}
	if runner == nil {
		return errors.New("runtime support is not configured")
	}
	start := runtimeusecase.GatewayStartOptions{
		ListenAddr:          options.listen,
		SmartContextEnabled: options.smartContext,
		PresidioEnabled:     options.presidio && !options.noPresidio,
	}
	var gateway *runtimeusecase.Gateway
	var release func() error
	providerLabel := "openai"

	if options.provider != "" {
		providerLabel = options.provider
		gateway, release, err = startProviderGateway(ctx, runner, profiles, options, start)
	} else {
		start.UpstreamURL = options.baseURL
		gateway, release, err = startCurrentGateway(ctx, runner, profiles, start)
	}
	if err != nil {
		if release != nil {
			_ = release()
		}
		return err
	}
	if release != nil {
		defer func() { _ = release() }()
	}

	for _, field := range [][2]string{
		{"Status", "listening"},
		{"Endpoint", gateway.Endpoint()},
		{"Provider", providerLabel},
		{"Stop", "Ctrl-C"},
	} {
		if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
			return closeGatewayAfterError(ctx, gateway, err)
		}
	}

	<-ctx.Done()
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return gateway.Close(closeCtx)
}

func startCurrentGateway(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	options runtimeusecase.GatewayStartOptions,
) (*runtimeusecase.Gateway, func() error, error) {
	if profiles == nil {
		gateway, err := runner.StartGatewayCurrent(ctx, options)
		return gateway, nil, err
	}
	target, active, err := profiles.ActiveLaunch(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !active {
		gateway, err := runner.StartGatewayCurrent(ctx, options)
		return gateway, nil, err
	}
	if target.AccountID != "" {
		gateway, err := runner.StartGatewayAccount(ctx, target.AccountID, options)
		return gateway, nil, err
	}
	if strings.TrimSpace(target.Name) == "" {
		return nil, nil, errors.New("active gateway profile metadata is incomplete")
	}
	release, err := profiles.AcquireLaunch(ctx, target.Name)
	if err != nil {
		return nil, nil, err
	}
	provider, err := launchRuntimeProvider(target)
	if err != nil {
		_ = release()
		return nil, nil, err
	}
	if provider.Kind != "" && options.UpstreamURL != "" {
		provider.APIURL = options.UpstreamURL
		options.UpstreamURL = ""
	}
	gateway, err := runner.StartGatewayProfile(ctx, target.CodexHome, provider, options)
	if err != nil {
		_ = release()
		return nil, nil, err
	}
	return gateway, release, nil
}

func startProviderGateway(
	ctx context.Context,
	runner *runtimeusecase.Runner,
	profiles launchProfiles,
	options gatewayOptions,
	start runtimeusecase.GatewayStartOptions,
) (*runtimeusecase.Gateway, func() error, error) {
	var target profilemodel.LaunchTarget
	found := false
	var err error
	if profiles != nil {
		target, found, err = profiles.ResolveProviderLaunch(ctx, options.provider, "")
		if err != nil {
			return nil, nil, err
		}
	}

	keys, err := runner.ProviderAPIKeys(options.provider, options.apiKey)
	if err != nil {
		return nil, nil, err
	}
	if len(keys) > 0 {
		home := ""
		name := options.provider + "-api-key"
		var release func() error
		if found {
			home, name = target.CodexHome, target.Name
			if target.Name != "" && target.AccountID == "" && profiles != nil {
				release, err = profiles.AcquireLaunch(ctx, target.Name)
				if err != nil {
					return nil, nil, err
				}
			}
		}
		provider, err := externalAPIKeyProvider(options.provider, name, options.baseURL)
		if err != nil {
			if release != nil {
				_ = release()
			}
			return nil, nil, err
		}
		gateway, err := runner.StartGatewayAPIKeys(ctx, home, provider, keys, start)
		if err != nil {
			if release != nil {
				_ = release()
			}
			return nil, nil, err
		}
		return gateway, release, nil
	}

	if found && target.Provider == options.provider &&
		(options.provider == anthropicProviderKind ||
			options.provider == copilotProviderKind ||
			options.provider == kiroProviderKind) {
		if profiles == nil {
			return nil, nil, errors.New("profile support is not configured")
		}
		release, err := profiles.AcquireLaunch(ctx, target.Name)
		if err != nil {
			return nil, nil, err
		}
		provider, err := launchRuntimeProvider(target)
		if err != nil {
			_ = release()
			return nil, nil, err
		}
		if options.baseURL != "" {
			provider.APIURL = options.baseURL
		}
		gateway, err := runner.StartGatewayProfile(ctx, target.CodexHome, provider, start)
		if err != nil {
			_ = release()
			return nil, nil, err
		}
		return gateway, release, nil
	}

	return nil, nil, gatewayProviderCredentialRequired(options.provider)
}

func parseGatewayArguments(arguments []string) (gatewayOptions, error) {
	var options gatewayOptions
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		switch argument {
		case "--smart-context":
			options.smartContext = true
			index++
			continue
		case "--presidio":
			if options.noPresidio {
				return gatewayOptions{}, errors.New("--presidio conflicts with --no-presidio")
			}
			options.presidio = true
			index++
			continue
		case "--no-presidio":
			if options.presidio {
				return gatewayOptions{}, errors.New("--no-presidio conflicts with --presidio")
			}
			options.noPresidio = true
			index++
			continue
		case "--help", "-h":
			return gatewayOptions{}, errors.New("usage: godex gateway [--listen ADDR] [--provider PROVIDER] [--base-url|--url URL] [--api-key KEY] [--smart-context] [--presidio|--no-presidio]")
		}

		if value, consumed, ok, err := namedOptionValue(arguments, index, "--listen"); ok {
			if err != nil {
				return gatewayOptions{}, err
			}
			options.listen = value
			index += consumed
			continue
		}
		if value, consumed, ok, err := namedOptionValue(arguments, index, "--provider"); ok {
			if err != nil {
				return gatewayOptions{}, err
			}
			normalized, err := normalizeExternalProvider(value)
			if err != nil {
				return gatewayOptions{}, err
			}
			options.provider = normalized
			index += consumed
			continue
		}
		if value, consumed, ok, err := namedSecretOptionValue(arguments, index, "--api-key"); ok {
			if err != nil {
				return gatewayOptions{}, err
			}
			options.apiKey = value
			index += consumed
			continue
		}
		baseValue, baseConsumed, baseOK, baseErr := namedOptionValue(arguments, index, "--base-url")
		if !baseOK {
			baseValue, baseConsumed, baseOK, baseErr = namedOptionValue(arguments, index, "--url")
		}
		if baseOK {
			if baseErr != nil {
				return gatewayOptions{}, baseErr
			}
			if err := validateCredentialFreeHTTPURL(baseValue, "--base-url"); err != nil {
				return gatewayOptions{}, err
			}
			options.baseURL = baseValue
			index += baseConsumed
			continue
		}
		return gatewayOptions{}, fmt.Errorf("unknown gateway option %q", argument)
	}
	if options.apiKey != "" && options.provider == "" {
		return gatewayOptions{}, errors.New("--api-key requires --provider")
	}
	return options, nil
}

func gatewayProviderCredentialRequired(kind string) error {
	switch kind {
	case anthropicProviderKind:
		return errors.New("godex gateway --provider anthropic requires a Claude profile, --api-key, or ANTHROPIC_API_KEY(S)")
	case deepSeekProviderKind:
		return errors.New("godex gateway --provider deepseek requires --api-key or DEEPSEEK_API_KEY(S)")
	case copilotProviderKind:
		return errors.New("godex gateway --provider copilot requires an imported Copilot profile, --api-key, or GITHUB_COPILOT_API_KEY(S)")
	case geminiProviderKind:
		return errors.New("godex gateway --provider gemini requires --api-key, GEMINI_API_KEY(S), or GOOGLE_API_KEY(S)")
	case kiroProviderKind:
		return errors.New("godex gateway --provider kiro requires an imported Kiro profile")
	default:
		return fmt.Errorf(providerShortcutNotImplementedFormat, kind)
	}
}

func closeGatewayAfterError(ctx context.Context, gateway *runtimeusecase.Gateway, primary error) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return errors.Join(primary, gateway.Close(closeCtx))
}

func gatewayProviderFromTarget(target profilemodel.LaunchTarget) (proxymodel.Provider, error) {
	return launchRuntimeProvider(target)
}
