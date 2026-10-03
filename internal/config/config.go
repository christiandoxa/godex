package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	HomeEnv                  = "GODEX_HOME"
	CodexBinEnv              = "GODEX_CODEX_BIN"
	AgyBinEnv                = "PRODEX_AGY_BIN"
	UpstreamEnv              = "GODEX_UPSTREAM_URL"
	CodexHomeEnv             = "CODEX_HOME"
	ProdexHomeEnv            = "PRODEX_HOME"
	ProdexSharedCodexHomeEnv = "PRODEX_SHARED_CODEX_HOME"
)

type Config struct {
	Home             string
	CodexBin         string
	AgyBin           string
	UpstreamURL      string
	CurrentCodexHome string
	SharedCodexHome  string
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
	sharedCodexHome, err := resolveSharedCodexHome()
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
		AgyBin:           resolveAgyBin(),
		UpstreamURL:      os.Getenv(UpstreamEnv),
		CurrentCodexHome: currentCodexHome,
		SharedCodexHome:  sharedCodexHome,
	}, nil
}

// LoadAntigravity resolves only the settings needed to launch native Antigravity.
func LoadAntigravity() (Config, error) {
	sharedCodexHome, err := resolveSharedCodexHome()
	if err != nil {
		return Config{}, err
	}
	return Config{AgyBin: resolveAgyBin(), SharedCodexHome: sharedCodexHome}, nil
}

func resolveAgyBin() string {
	if binary := os.Getenv(AgyBinEnv); binary != "" {
		return binary
	}
	return "agy"
}

func resolveSharedCodexHome() (string, error) {
	shared, found := os.LookupEnv(ProdexSharedCodexHomeEnv)
	if !found {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve shared Codex home: %w", err)
		}
		return filepath.Abs(filepath.Join(userHome, ".codex"))
	}
	if filepath.IsAbs(shared) {
		return filepath.Clean(shared), nil
	}
	root, found := os.LookupEnv(ProdexHomeEnv)
	if !found {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Prodex home: %w", err)
		}
		root = filepath.Join(userHome, ".prodex")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve Prodex home: %w", err)
	}
	return filepath.Join(root, shared), nil
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
