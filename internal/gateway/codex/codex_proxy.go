package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func (process *CodexProcess) CheckProxySupport(ctx context.Context) error {
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
	configArguments, err := (RuntimeConfig{
		ChatGPTBaseURL: "http://127.0.0.1:1/backend-api",
		OpenAIBaseURL:  "http://127.0.0.1:1/backend-api/prodex",
	}).arguments()
	if err != nil {
		return err
	}
	arguments := append([]string{"--strict-config"}, configArguments...)
	arguments = append(arguments, "--version")
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
		return nil, errors.New("codex arguments cannot override Godex proxy base URLs")
	}
	configArguments, err := (RuntimeConfig{
		ChatGPTBaseURL: endpoint + "/backend-api",
		OpenAIBaseURL:  endpoint + "/backend-api/prodex",
	}).arguments()
	if err != nil {
		return nil, err
	}
	return append(configArguments, arguments...), nil
}

func containsProxyOverride(arguments []string) bool {
	for index, argument := range arguments {
		value := ""
		switch {
		case argument == "-c" || argument == "--config":
			if index+1 < len(arguments) {
				value = arguments[index+1]
			}
		case strings.HasPrefix(argument, "-c"):
			value = strings.TrimPrefix(argument, "-c")
		case strings.HasPrefix(argument, "--config="):
			value = strings.TrimPrefix(argument, "--config=")
		default:
			continue
		}
		key, _, _ := strings.Cut(strings.TrimSpace(value), "=")
		key = strings.TrimSpace(key)
		if key == "chatgpt_base_url" || key == "openai_base_url" {
			return true
		}
	}
	return false
}
