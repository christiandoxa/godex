package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (process *CodexProcess) CheckProxySupport(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	version, err := process.Version(ctx)
	if err != nil {
		return err
	}
	if err := requireSupportedVersion(version); err != nil {
		return err
	}
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	home, err := os.MkdirTemp("", "godex-codex-probe-")
	if err != nil {
		return fmt.Errorf("create Codex capability probe home: %w", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(home)
	configArguments := managedModelArguments("http://127.0.0.1:1/backend-api/godex")
	arguments := append([]string{"--strict-config"}, configArguments...)
	// Stdio EOF validates configuration and exits without accepting any work.
	arguments = append(arguments, "exec-server", "--listen", "stdio")
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir = home
	command.Env = environmentWith("CODEX_HOME", home)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("official Codex CLI lacks required proxy configuration; upgrade Codex")
	}
	return nil
}

func (process *CodexProcess) RunThroughProxy(ctx context.Context, codexHome, endpoint string, arguments []string) error {
	return process.runThroughProxy(ctx, codexHome, endpoint, arguments, "", false)
}

// RunThroughProxyWithSessionServer starts the managed child with a private
// Codex app-server companion. Super uses this to keep live session control in
// the same isolated home as the foreground TUI.
func (process *CodexProcess) RunThroughProxyWithSessionServer(
	ctx context.Context,
	codexHome, endpoint string,
	arguments []string,
	provider string,
) error {
	return process.runThroughProxy(ctx, codexHome, endpoint, arguments, provider, true)
}

func (process *CodexProcess) RunThroughProxyProvider(
	ctx context.Context,
	codexHome, endpoint string,
	arguments []string,
	provider string,
) error {
	return process.runThroughProxy(ctx, codexHome, endpoint, arguments, provider, false)
}

func (process *CodexProcess) runThroughProxy(
	ctx context.Context,
	codexHome, endpoint string,
	arguments []string,
	provider string,
	withSessionServer bool,
) error {
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	if err := secureCodexHomeWithShared(codexHome, process.sharedCodexHome); err != nil {
		return err
	}
	commandServer := codexCommandServerSubcommand(arguments)
	companion := withSessionServer && sessionAppServerEligible(arguments)
	switch provider {
	case "local":
		arguments, err = localProxyArguments(endpoint, arguments)
	case "openai-compatible":
		arguments, err = openAICompatibleProxyArguments(endpoint, arguments)
	case "anthropic", "copilot", "deepseek", "gemini", "kiro":
		arguments, err = externalProviderProxyArguments(endpoint, arguments, provider)
	default:
		arguments, err = proxyArguments(endpoint, arguments)
	}
	if err != nil {
		return err
	}
	if !commandServer {
		arguments = codexTUIArguments(arguments)
		resetTerminalKeyboardEnhancementBestEffort(process.terminal.Stdout)
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, codexHome)
	if err != nil {
		return err
	}
	defer release()
	command := terminalCommand(ctx, binary, arguments)
	command.Env = proxyChildEnvironment(codexHome, provider, process.sharedCodexHome)
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if companion {
		return process.runWithSessionAppServer(ctx, binary, codexHome, command.Env, command, arguments)
	}
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func proxyChildEnvironment(codexHome, provider, sharedCodexHome string) []string {
	environment := codexThreadIndexEnvironment(codexHome, sharedCodexHome)
	environment = prependCodexHomeBin(environment, codexHome)
	if strings.TrimSpace(provider) == "" {
		return environment
	}
	blocked := map[string]bool{
		"OPENAI_API_KEYS":         true,
		"OPENAI_API_KEY":          true,
		"ANTHROPIC_API_KEYS":      true,
		"ANTHROPIC_API_KEY":       true,
		"DEEPSEEK_API_KEYS":       true,
		"DEEPSEEK_API_KEY":        true,
		"GEMINI_API_KEYS":         true,
		"GEMINI_API_KEY":          true,
		"GOOGLE_API_KEYS":         true,
		"GOOGLE_API_KEY":          true,
		"GITHUB_COPILOT_API_KEYS": true,
		"GITHUB_COPILOT_API_KEY":  true,
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found && blocked[strings.ToUpper(key)] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func prependCodexHomeBin(environment []string, codexHome string) []string {
	bin := filepath.Join(codexHome, "bin")
	info, err := os.Stat(bin)
	if err != nil || !info.IsDir() {
		return environment
	}
	filtered := make([]string, 0, len(environment)+1)
	pathSet := false
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && (strings.EqualFold(key, "PRODEX_RTK_AUTO_WRAP_DEPTH") ||
			strings.EqualFold(key, "PRODEX_RTK_DISABLE_AUTO_WRAP")) {
			continue
		}
		if found && strings.EqualFold(key, "PATH") {
			filtered = append(filtered, "PATH="+bin+string(os.PathListSeparator)+value)
			pathSet = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !pathSet {
		filtered = append(filtered, "PATH="+bin)
	}
	return filtered
}

func openAICompatibleProxyArguments(endpoint string, arguments []string) ([]string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("proxy endpoint is required")
	}
	if containsProxyOverride(arguments) {
		return nil, errors.New("codex arguments cannot override Godex routing or credential storage")
	}
	if err := validateRuntimeURL(endpoint); err != nil {
		return nil, err
	}
	values := []string{
		"cli_auth_credentials_store=\"file\"",
		"model_provider=\"godex-openai-compatible\"",
		"model_providers.godex-openai-compatible.name=\"OpenAI-compatible through Godex\"",
		"model_providers.godex-openai-compatible.base_url=" + strconv.Quote(endpoint+"/v1"),
		"model_providers.godex-openai-compatible.wire_api=\"responses\"",
		"model_providers.godex-openai-compatible.requires_openai_auth=true",
		"model_providers.godex-openai-compatible.supports_websockets=false",
	}
	managed := make([]string, 0, len(values)*2)
	for _, value := range values {
		managed = append(managed, "-c", value)
	}
	return scopeModelArguments(arguments, managed), nil
}

func externalProviderProxyArguments(endpoint string, arguments []string, provider string) ([]string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	provider = strings.ToLower(strings.TrimSpace(provider))
	if endpoint == "" {
		return nil, errors.New("proxy endpoint is required")
	}
	if containsProxyOverride(arguments) {
		return nil, errors.New("codex arguments cannot override Godex routing or credential storage")
	}
	if err := validateRuntimeURL(endpoint); err != nil {
		return nil, err
	}
	names := map[string]string{
		"anthropic": "Anthropic",
		"copilot":   "OpenAI",
		"deepseek":  "DeepSeek",
		"gemini":    "Azure",
		"kiro":      "Azure",
	}
	name, ok := names[provider]
	if !ok {
		return nil, errors.New("runtime provider identity is unsupported")
	}
	id := "godex-" + provider
	baseURL := endpoint + "/backend-api/godex"
	values := []string{
		"cli_auth_credentials_store=<redacted>",
		"model_provider=" + strconv.Quote(id),
		"model_providers." + id + ".name=" + strconv.Quote(name),
		"model_providers." + id + ".base_url=" + strconv.Quote(baseURL),
		"model_providers." + id + ".wire_api=\"responses\"",
		"model_providers." + id + ".requires_openai_auth=true",
		"model_providers." + id + ".supports_websockets=false",
	}
	managed := make([]string, 0, len(values)*2)
	for _, value := range values {
		managed = append(managed, "-c", value)
	}
	return scopeModelArguments(arguments, managed), nil
}

func localProxyArguments(endpoint string, arguments []string) ([]string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("proxy endpoint is required")
	}
	if containsProxyOverride(arguments) {
		return nil, errors.New("codex arguments cannot override Godex routing or credential storage")
	}
	if err := validateRuntimeURL(endpoint); err != nil {
		return nil, err
	}
	managed := managedModelArguments(endpoint + "/v1")
	for index := 0; index+1 < len(managed); index += 2 {
		value := managed[index+1]
		value = strings.ReplaceAll(value, "godex-openai", "godex-local")
		value = strings.ReplaceAll(value, "OpenAI through Godex", "Godex Local")
		managed[index+1] = value
	}
	filtered := managed[:0]
	for index := 0; index+1 < len(managed); index += 2 {
		if strings.Contains(managed[index+1], ".supports_standalone_web_search=") {
			continue
		}
		filtered = append(filtered, managed[index], managed[index+1])
	}
	return scopeModelArguments(arguments, filtered), nil
}

func proxyArguments(endpoint string, arguments []string) ([]string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("proxy endpoint is required")
	}
	if containsProxyOverride(arguments) {
		return nil, errors.New("codex arguments cannot override Godex routing or credential storage")
	}
	if err := validateRuntimeURL(endpoint); err != nil {
		return nil, err
	}
	return scopeModelArguments(arguments, managedModelArguments(endpoint+"/backend-api/godex")), nil
}

func containsProxyOverride(arguments []string) bool {
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			return false
		}
		if argument == "--oss" || strings.HasPrefix(argument, "--oss=") || argument == "--local-provider" || strings.HasPrefix(argument, "--local-provider=") {
			return true
		}
		if argument == "--remote" || strings.HasPrefix(argument, "--remote=") || argument == "--remote-auth-token-env" || strings.HasPrefix(argument, "--remote-auth-token-env=") {
			return true
		}
		value, consumed, ok := configArgument(arguments, index)
		if ok && ownedConfigKey(value) {
			return true
		}
		index += consumed
	}
	return false
}

// Proxy only model traffic; native Codex account/bootstrap endpoints remain HTTPS.
func managedModelArguments(baseURL string) []string {
	values := []string{
		`cli_auth_credentials_store="file"`,
		`model_provider="godex-openai"`,
		`model_providers.godex-openai.name="OpenAI through Godex"`,
		"model_providers.godex-openai.base_url=" + strconv.Quote(baseURL),
		`model_providers.godex-openai.wire_api="responses"`,
		`model_providers.godex-openai.requires_openai_auth=true`,
		`model_providers.godex-openai.supports_websockets=false`,
		`model_providers.godex-openai.supports_standalone_web_search=true`,
	}
	arguments := make([]string, 0, 2*len(values))
	for _, value := range values {
		arguments = append(arguments, "-c", value)
	}
	return arguments
}
