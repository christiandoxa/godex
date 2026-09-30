package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	HomeEnv      = "GODEX_HOME"
	CodexBinEnv  = "GODEX_CODEX_BIN"
	UpstreamEnv  = "GODEX_UPSTREAM_URL"
	CodexHomeEnv = "CODEX_HOME"
)

type Config struct {
	Home             string
	CodexBin         string
	UpstreamURL      string
	CurrentCodexHome string
}

func Load() (Config, error) {
	home, err := resolveHome()
	if err != nil {
		return Config{}, err
	}

	currentCodexHome, err := resolveCurrentCodexHome()
	if err != nil {
		return Config{}, err
	}

	codexBin := os.Getenv(CodexBinEnv)
	if codexBin == "" {
		codexBin = "codex"
	}

	return Config{
		Home:             filepath.Clean(home),
		CodexBin:         codexBin,
		UpstreamURL:      os.Getenv(UpstreamEnv),
		CurrentCodexHome: currentCodexHome,
	}, nil
}

func resolveHome() (string, error) {
	value := os.Getenv(HomeEnv)
	source := HomeEnv
	if value == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		value = filepath.Join(userHome, ".godex")
		source = "user home"
	}

	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", source, err)
	}
	clean := filepath.Clean(absolute)
	if clean == filepath.Dir(clean) {
		return "", fmt.Errorf("%s must not be the filesystem root", source)
	}
	return clean, nil
}

func resolveCurrentCodexHome() (string, error) {
	value := os.Getenv(CodexHomeEnv)
	if value == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Codex home: %w", err)
		}
		value = filepath.Join(userHome, ".codex")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve Codex home: %w", err)
	}
	clean := filepath.Clean(absolute)
	if clean == filepath.Dir(clean) {
		return "", errors.New("CODEX_HOME must not be the filesystem root")
	}
	return clean, nil
}
