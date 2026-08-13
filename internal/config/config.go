package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	HomeEnv     = "GODEX_HOME"
	CodexBinEnv = "GODEX_CODEX_BIN"
	UpstreamEnv = "GODEX_UPSTREAM_URL"
)

type Config struct {
	Home        string
	CodexBin    string
	UpstreamURL string
}

func Load() (Config, error) {
	home, err := resolveHome()
	if err != nil {
		return Config{}, err
	}

	codexBin := os.Getenv(CodexBinEnv)
	if codexBin == "" {
		codexBin = "codex"
	}

	return Config{
		Home:        filepath.Clean(home),
		CodexBin:    codexBin,
		UpstreamURL: os.Getenv(UpstreamEnv),
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
