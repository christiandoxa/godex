package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	localProviderID              = "godex-local"
	localProviderName            = "Godex Local"
	localProviderConfigPrefix    = "model_providers." + localProviderID
	localDefaultModel            = "unsloth/qwen3.5-35b-a3b"
	localDefaultContextWindow    = uint64(16_384)
	localDefaultAutoCompactLimit = uint64(14_000)
)

type LocalProviderConfig struct {
	URL                   string
	Model                 string
	ContextWindow         *uint64
	AutoCompactTokenLimit *uint64
}

func (runner *Runner) RunLocalProvider(
	ctx context.Context,
	selector string,
	config LocalProviderConfig,
	args []string,
) (err error) {
	account, err := runner.activeAccount(ctx, selector)
	if err != nil {
		if selector == "" && strings.TrimSpace(runner.currentHome) != "" {
			return runner.RunLocalProviderHome(ctx, runner.currentHome, config, args)
		}
		return err
	}
	if leases, ok := runner.accounts.(interface {
		AcquireProfiles(context.Context, []string) (func() error, error)
	}); ok {
		release, err := leases.AcquireProfiles(ctx, []string{account.ID})
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, release()) }()
	}
	return runner.RunLocalProviderHome(ctx, runner.accounts.CodexHome(account.ID), config, args)
}

func (runner *Runner) RunLocalProviderHome(
	ctx context.Context,
	codexHome string,
	config LocalProviderConfig,
	args []string,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	prepared, err := localProviderArguments(config, args)
	if err != nil {
		return err
	}
	return runner.process.Run(ctx, home, prepared)
}

func localProviderArguments(config LocalProviderConfig, arguments []string) ([]string, error) {
	baseURL, err := localProviderBaseURL(config.URL)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = localDefaultModel
	}
	contextWindow := localDefaultContextWindow
	if config.ContextWindow != nil && *config.ContextWindow > 1 {
		contextWindow = *config.ContextWindow
	}
	autoCompact := localDefaultAutoCompactLimit
	if config.AutoCompactTokenLimit != nil && *config.AutoCompactTokenLimit > 0 {
		autoCompact = *config.AutoCompactTokenLimit
	}
	if autoCompact >= contextWindow {
		autoCompact = contextWindow - 1
	}
	entries := []string{
		"model_provider=" + strconv.Quote(localProviderID),
		"model=" + strconv.Quote(model),
		localProviderConfigPrefix + ".name=" + strconv.Quote(localProviderName),
		localProviderConfigPrefix + ".base_url=" + strconv.Quote(baseURL),
		localProviderConfigPrefix + `.wire_api="responses"`,
		localProviderConfigPrefix + ".requires_openai_auth=true",
		localProviderConfigPrefix + ".supports_websockets=false",
		"model_context_window=" + strconv.FormatUint(contextWindow, 10),
		"model_auto_compact_token_limit=" + strconv.FormatUint(autoCompact, 10),
		`model_reasoning_summary="none"`,
		`web_search="disabled"`,
		"features.apps=false",
		"features.js_repl=false",
		"features.image_generation=false",
	}
	managed := make([]string, 0, len(entries)*2+len(arguments))
	for _, entry := range entries {
		managed = append(managed, "-c", entry)
	}
	return append(managed, arguments...), nil
}

func localProviderBaseURL(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("--url requires a value")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("invalid --url: expected an absolute http(s) URL with host and no credentials, query, or fragment")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ApplyProviderSelectionLimits(provider *proxymodel.Provider, contextWindow, autoCompact *uint64) error {
	if provider == nil {
		return errors.New("runtime provider is required")
	}
	contextValue := uint64(provider.ContextWindow)
	if contextWindow != nil && *contextWindow > 1 {
		contextValue = *contextWindow
	}
	compactValue := uint64(provider.AutoCompactLimit)
	if autoCompact != nil && *autoCompact > 0 {
		compactValue = *autoCompact
	}
	if contextValue == 0 {
		return fmt.Errorf("runtime provider context window is invalid")
	}
	if compactValue >= contextValue {
		compactValue = contextValue - 1
	}
	provider.ContextWindow = int64(contextValue)
	provider.AutoCompactLimit = int64(compactValue)
	return nil
}
