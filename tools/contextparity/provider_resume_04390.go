package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type providerIdentityProbe04390 struct {
	provider string
	reject   bool
}

type providerIdentityRun04390 struct {
	exit               int
	stdout, diagnostic string
	started, homeMatch bool
	arguments          []string
}

type providerIdentityResult04390 struct {
	Provider string `json:"provider"`
	Prodex   string `json:"prodex"`
	Godex    string `json:"godex"`
}

func runProviderIdentityProbes04390(input options, root, fake string) ([]providerIdentityResult04390, error) {
	var results []providerIdentityResult04390
	for _, probe := range []providerIdentityProbe04390{
		{"openai", false}, {"openai-responses", false}, {"openai-compatible", false}, {"openai_compatible", false},
		{"prodex-openai-governed-http", false}, {"prodex-openai", true}, {"prodex-openai-compatible", true}, {"synthetic-unknown", true},
	} {
		for _, product := range []struct{ name, binary string }{{"prodex", input.prodex}, {"godex", input.godex}} {
			directory := filepath.Join(root, "provider-identities", probe.provider, product.name)
			if err := runProviderIdentityProbe04390(product.binary, product.name, directory, fake, probe); err != nil {
				return nil, fmt.Errorf("%s shared provider %s: %w", product.name, probe.provider, err)
			}
		}
		results = append(results, providerIdentityResult04390{probe.provider, "PASS", "PASS"})
	}
	return results, nil
}

func runProviderIdentityProbe04390(binary, product, root, fake string, probe providerIdentityProbe04390) error {
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	auth := filepath.Join(profile, "auth.json")
	if err := os.WriteFile(auth, []byte(superResumeAuth04390), 0o600); err != nil {
		return err
	}
	if err := prepareProfile(binary, product, root, fake); err != nil {
		return err
	}
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		return err
	}
	sessions := filepath.Join(shared, "sessions")
	if err := os.Mkdir(sessions, 0o700); err != nil {
		return err
	}
	metadata, err := nativeCLISessionMetadata04390(root, nativeSession04390, probe.provider)
	if err != nil {
		return err
	}
	rollout := filepath.Join(sessions, "rollout-2026-10-23T00-00-00-"+nativeSession04390+".jsonl")
	if err := os.WriteFile(rollout, metadata, 0o600); err != nil {
		return err
	}
	record := filepath.Join(root, "child")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "run", "--skip-quota-check", "--no-auto-rotate", "--profile", "main", "exec", "resume", nativeSession04390, "continue")
	command.Dir = root
	command.Env = append(probeEnvironment(root, product, fake, record), "PRODEX_SHARED_CODEX_HOME="+shared)
	stdout, launchErr := command.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	result := providerIdentityRun04390{stdout: string(stdout)}
	if launchErr != nil {
		var exit *exec.ExitError
		if !errors.As(launchErr, &exit) {
			return launchErr
		}
		result.exit, result.diagnostic = exit.ExitCode(), string(exit.Stderr)
	}
	if data, err := os.ReadFile(record); err == nil {
		result.started = true
		result.arguments = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		home, err := os.ReadFile(record + ".home")
		result.homeMatch = err == nil && string(home) == profile
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := verifyProviderIdentityRun04390(probe, result); err != nil {
		return err
	}
	for path, want := range map[string]string{auth: superResumeAuth04390, rollout: string(metadata)} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			return fmt.Errorf("provider resolution changed a profile or rollout file: %v", err)
		}
	}
	return verifyProfileFiles(root)
}

func verifyProviderIdentityRun04390(probe providerIdentityProbe04390, result providerIdentityRun04390) error {
	if probe.reject {
		if result.exit != 1 || result.stdout != "" || result.started ||
			!strings.Contains(result.diagnostic, "unsupported provider identity") || strings.Contains(result.diagnostic, probe.provider) {
			return errors.New("unsupported provider did not fail safely before child launch")
		}
		return nil
	}
	if result.exit != 0 || result.stdout != "context-probe-ok\n" || !result.started || !result.homeMatch ||
		!slices.Equal(result.arguments, []string{"exec", "resume", nativeSession04390, "continue"}) {
		return errors.New("accepted provider lost exact native arguments, output or selected home")
	}
	return nil
}
