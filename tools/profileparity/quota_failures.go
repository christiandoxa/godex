package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type quotaFailureCase struct {
	name, config, body string
	status             int
}

type quotaFailureResult struct {
	code           int
	stdout, stderr string
	requests       []startupProbeRequest
	unchanged      bool
}

func quotaFailureCases() []quotaFailureCase {
	return []quotaFailureCase{
		{"malformed_config", "enforce_residency = [\n", `{}`, 200},
		{"http_failure", "", `{"error":"synthetic_quota_failure"}`, 503},
		{"invalid_json", "", `synthetic_quota_failure`, 200},
	}
}

func checkQuotaFailures(root string, opts cliOptions) ([]stepResult, error) {
	var results []stepResult
	for _, probe := range quotaFailureCases() {
		for _, mode := range []string{"once", "raw", "all"} {
			step := stepResult{Name: "quota_" + probe.name + "_" + mode}
			for _, product := range []struct{ name, binary string }{{"prodex", opts.prodex}, {"godex", opts.godex}} {
				result, err := runQuotaFailure(root, product.name, product.binary, probe, mode)
				if err != nil {
					return nil, fmt.Errorf("%s %s: %w", product.name, step.Name, err)
				}
				if err := verifyQuotaFailure(probe, mode, result); err != nil {
					return nil, fmt.Errorf("%s %s: %w", product.name, step.Name, err)
				}
				if product.name == "prodex" {
					step.ProdexExit = result.code
				} else {
					step.GodexExit = result.code
				}
				step.Requests = len(result.requests)
			}
			results = append(results, step)
		}
	}
	return results, nil
}

func verifyQuotaFailure(probe quotaFailureCase, mode string, result quotaFailureResult) error {
	wantCode, wantRequests := 1, 1
	if mode == "all" {
		wantCode = 0
	}
	if probe.config != "" {
		wantRequests = 0
	}
	if result.code != wantCode || len(result.requests) != wantRequests || !result.unchanged {
		return fmt.Errorf("exit=%d requests=%d unchanged=%t; want %d/%d/true", result.code, len(result.requests), result.unchanged, wantCode, wantRequests)
	}
	if mode == "all" {
		if result.stderr != "" || !strings.Contains(result.stdout, "main") {
			return fmt.Errorf("aggregate lost error row or wrote stderr")
		}
	} else if result.stdout != "" || result.stderr == "" {
		return fmt.Errorf("profile failure did not use stderr exclusively")
	}
	if strings.Contains(result.stdout+result.stderr, "synthetic-access") {
		return fmt.Errorf("diagnostic exposed synthetic credential")
	}
	for _, request := range result.requests {
		if request != (startupProbeRequest{method: "GET", path: startupUsagePath, authValid: true}) {
			return fmt.Errorf("quota failure changed request identity or route")
		}
	}
	return nil
}

func runQuotaFailure(root, product, binary string, probe quotaFailureCase, mode string) (quotaFailureResult, error) {
	directory := filepath.Join(root, "quota-failures", product, probe.name+"_"+mode)
	profile, state := filepath.Join(directory, "profile"), filepath.Join(directory, "state")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return quotaFailureResult{}, err
	}
	auth := filepath.Join(profile, "auth.json")
	config := filepath.Join(profile, "config.toml")
	if err := os.WriteFile(auth, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-access"}}`), 0o600); err != nil {
		return quotaFailureResult{}, err
	}
	if err := os.WriteFile(config, []byte(probe.config), 0o600); err != nil {
		return quotaFailureResult{}, err
	}
	fixture := profileFixture{binary: binary, home: directory, env: startupEnvironment(directory, profile, state)}
	code, _, err := invokeProfileCommand(fixture, []string{"profile", "add", "main", "--codex-home", profile, "--activate"})
	if err != nil || code != 0 {
		return quotaFailureResult{}, fmt.Errorf("register synthetic profile: exit=%d err=%v", code, err)
	}
	stateName := "profiles.json"
	if product == "prodex" {
		stateName = "state.json"
	}
	files := []string{auth, config, filepath.Join(state, stateName)}
	before := make([][]byte, len(files))
	for index, path := range files {
		before[index], err = os.ReadFile(path)
		if err != nil {
			return quotaFailureResult{}, err
		}
	}
	usage := newUsageServer(probe.status, probe.body, "")
	defer usage.Close()
	args := []string{"quota", "--base-url", usage.server.URL + "/backend-api"}
	if mode == "all" {
		args = append(args, "--all", "--once")
	} else {
		args = append(args, "--profile", "main", "--"+mode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, fixture.env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	_ = command.Run()
	if ctx.Err() != nil || command.ProcessState == nil {
		return quotaFailureResult{}, fmt.Errorf("quota subprocess failed to complete")
	}
	result := quotaFailureResult{command.ProcessState.ExitCode(), stdout.String(), stderr.String(), usage.requests(), true}
	for index, path := range files {
		after, readErr := os.ReadFile(path)
		result.unchanged = result.unchanged && readErr == nil && bytes.Equal(before[index], after)
	}
	return result, nil
}
