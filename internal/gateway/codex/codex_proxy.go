package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	configArguments := managedModelArguments("http://127.0.0.1:1/backend-api/prodex")
	arguments := append([]string{"--strict-config"}, configArguments...)
	// Stdio EOF validates configuration and exits without accepting any work.
	arguments = append(arguments, "exec-server", "--listen", "stdio")
	command := exec.CommandContext(ctx, binary, arguments...)
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
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	if err := secureCodexHome(codexHome); err != nil {
		return err
	}
	arguments, err = proxyArguments(endpoint, arguments)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = environmentWith("CODEX_HOME", codexHome)
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
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
	return scopeModelArguments(arguments, managedModelArguments(endpoint+"/backend-api/prodex")), nil
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
