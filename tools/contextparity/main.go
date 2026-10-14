// contextparity compares native context configuration forwarding with the
// official Prodex 0.437.12 executable and a Godex candidate.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	oracleVersion      = "prodex 0.437.12"
	oracleSourceCommit = "e9d5a16f9abec6976cd670021c1e7f0b04a67b69"
	oracleSHA256       = "ecab17893af7142304072c74c7af723347712a4fa5d055142e7c5035727d9d63"
	probeProfileConfig = "model = \"gpt-6.1-sol\"\nmodel_context_window = 640000\nmodel_auto_compact_token_limit = 520000\n"
)

type options struct {
	prodex       string
	godex        string
	prodexSource string
	godexSource  string
	godexCommit  string
}

type probe struct {
	Name          string
	Arguments     []string
	Model         string
	ContextWindow string
	CompactLimit  string
	ProfileOnly   bool
}

type childArgs struct {
	Model         string `json:"model,omitempty"`
	ContextWindow string `json:"context_window,omitempty"`
	CompactLimit  string `json:"compact_limit,omitempty"`
}

type caseResult struct {
	Name   string    `json:"name"`
	Prodex childArgs `json:"prodex"`
	Godex  childArgs `json:"godex"`
}

type report struct {
	Status          string       `json:"status"`
	Oracle          string       `json:"oracle"`
	OracleSource    string       `json:"oracle_source"`
	CandidateSource string       `json:"candidate_source"`
	Cases           []caseResult `json:"cases"`
	NegativeControl string       `json:"negative_control"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if runtime.GOOS == "windows" {
		return errors.New("contextparity requires a POSIX shell fixture")
	}
	options, err := parseOptions()
	if err != nil {
		return err
	}
	if err := verifyOracle(options.prodex); err != nil {
		return err
	}
	if err := verifyContextProvenance(options); err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "godex-context-parity-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	fake, err := writeFakeCodex(root)
	if err != nil {
		return err
	}
	probes := contextProbes()
	oracleRoot := filepath.Join(root, "prodex")
	candidateRoot := filepath.Join(root, "godex")
	if err := prepareProfile(options.prodex, "prodex", oracleRoot, fake); err != nil {
		return fmt.Errorf("prepare Prodex profile: %w", err)
	}
	if err := prepareProfile(options.godex, "godex", candidateRoot, fake); err != nil {
		return fmt.Errorf("prepare Godex profile: %w", err)
	}
	results := make([]caseResult, 0, len(probes))
	for _, item := range probes {
		prodexArgs, err := runProbe(options.prodex, "prodex", oracleRoot, fake, item)
		if err != nil {
			return fmt.Errorf("%s Prodex: %w", item.Name, err)
		}
		godexArgs, err := runProbe(options.godex, "godex", candidateRoot, fake, item)
		if err != nil {
			return fmt.Errorf("%s Godex: %w", item.Name, err)
		}
		want := childArgs{Model: item.Model, ContextWindow: item.ContextWindow, CompactLimit: item.CompactLimit}
		if item.ProfileOnly {
			want = childArgs{Model: ""}
		}
		if err := validateProbe(prodexArgs, want); err != nil {
			return fmt.Errorf("%s Prodex: %w", item.Name, err)
		}
		if err := validateProbe(godexArgs, want); err != nil {
			return fmt.Errorf("%s Godex: %w", item.Name, err)
		}
		results = append(results, caseResult{Name: item.Name, Prodex: prodexArgs, Godex: godexArgs})
	}
	for _, root := range []string{oracleRoot, candidateRoot} {
		if err := verifyProfileFiles(root); err != nil {
			return err
		}
	}
	if err := checkNegativeControl(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report{
		Status: "PASS", Oracle: oracleVersion, OracleSource: oracleSourceCommit,
		CandidateSource: options.godexCommit, Cases: results,
		NegativeControl: "PASS: cached maximum promotion is rejected",
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func parseOptions() (options, error) {
	var result options
	flags := flag.NewFlagSet("contextparity", flag.ContinueOnError)
	flags.StringVar(&result.prodex, "prodex", "", "official Prodex 0.437.12 binary")
	flags.StringVar(&result.godex, "godex", "", "Godex candidate binary")
	flags.StringVar(&result.prodexSource, "prodex-source", "", "tagged Prodex 0.437.12 clean source")
	flags.StringVar(&result.godexSource, "godex-source", "", "clean Godex candidate source")
	flags.StringVar(&result.godexCommit, "godex-commit", "", "full expected Godex commit SHA")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return result, err
	}
	if result.prodex == "" || result.godex == "" || result.prodexSource == "" ||
		result.godexSource == "" || len(result.godexCommit) != 40 {
		return result, errors.New("usage: contextparity --prodex BIN --godex BIN --prodex-source DIR --godex-source DIR --godex-commit FULL_SHA")
	}
	return result, nil
}

func verifyOracle(path string) error {
	digest, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("read Prodex binary: %w", err)
	}
	if digest != oracleSHA256 {
		return fmt.Errorf("Prodex binary digest mismatch: %s", digest)
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != oracleVersion {
		return fmt.Errorf("unexpected Prodex version: %q: %v", output, err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func contextProbes() []probe {
	return []probe{
		{
			Name: "explicit_sol", Model: "gpt-6.1-sol", ContextWindow: "500000", CompactLimit: "430000",
			Arguments: []string{"run", "--skip-quota-check", "--no-auto-rotate", "--profile", "main", "--model", "gpt-6.1-sol", "-c", "model_context_window=500000", "-c", "model_auto_compact_token_limit=430000", "exec", "hello"},
		},
		{
			Name: "model_switch_luna", Model: "gpt-6-luna", ContextWindow: "300000", CompactLimit: "280000",
			Arguments: []string{"run", "--skip-quota-check", "--no-auto-rotate", "--profile", "main", "--model", "gpt-6-luna", "-c", "model_context_window=300000", "-c", "model_auto_compact_token_limit=280000", "exec", "hello"},
		},
		{
			Name: "profile_values", ProfileOnly: true,
			Arguments: []string{"run", "--skip-quota-check", "--no-auto-rotate", "--profile", "main", "exec", "hello"},
		},
	}
}

func prepareProfile(binary, name, root, fake string) error {
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	config := []byte(probeProfileConfig)
	for _, file := range []struct {
		name string
		data []byte
	}{
		{name: "config.toml", data: config},
		{name: "models_cache.json", data: []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000,"max_context_window":872000}]}`)},
	} {
		if err := os.WriteFile(filepath.Join(profile, file.name), file.data, 0o600); err != nil {
			return err
		}
	}
	environment := probeEnvironment(root, name, fake, "")
	return runCommand(context.Background(), root, binary, environment, "profile", "add", "main", "--codex-home", profile, "--activate")
}

func runProbe(binary, name, root, fake string, item probe) (childArgs, error) {
	record := filepath.Join(root, item.Name+"-args")
	environment := probeEnvironment(root, name, fake, record)
	if err := runCommand(context.Background(), root, binary, environment, item.Arguments...); err != nil {
		return childArgs{}, err
	}
	content, err := os.ReadFile(record)
	if err != nil {
		return childArgs{}, fmt.Errorf("read child argv: %w", err)
	}
	childHome, err := os.ReadFile(record + ".home")
	if err != nil || string(childHome) != filepath.Join(root, "profile") {
		return childArgs{}, fmt.Errorf("child CODEX_HOME = %q, want %q", childHome, filepath.Join(root, "profile"))
	}
	config, err := os.ReadFile(record + ".config")
	if err != nil || string(config) != probeProfileConfig {
		return childArgs{}, fmt.Errorf("child profile config = %q, want %q", config, probeProfileConfig)
	}
	return parseChildArgs(strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")), nil
}

func probeEnvironment(root, name, fake, record string) []string {
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(root, "home"),
		"CODEX_HOME=" + filepath.Join(root, "ambient-codex"),
		"TMPDIR=" + root,
		"TEMP=" + root,
		"TMP=" + root,
		"NO_COLOR=1", "CI=1", "TERM=dumb", "PRODEX_NO_UPDATE_CHECK=1", "GODEX_NO_UPDATE_CHECK=1",
		"PRODEX_CODEX_BIN=" + fake, "GODEX_CODEX_BIN=" + fake,
		"CONTEXT_PARITY_ARGS=" + record,
		"CONTEXT_PARITY_HOME=" + record + ".home",
		"CONTEXT_PARITY_CONFIG=" + record + ".config",
	}
	if name == "prodex" {
		environment = append(environment, "PRODEX_HOME="+root+"/state")
	} else {
		environment = append(environment, "GODEX_HOME="+root+"/state")
	}
	return environment
}

func writeFakeCodex(root string) (string, error) {
	path := filepath.Join(root, "codex")
	body := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'codex 0.162.0\\n'; exit 0; fi\nprintf '%s\\n' \"$@\" > \"$CONTEXT_PARITY_ARGS\"\nprintf '%s' \"$CODEX_HOME\" > \"$CONTEXT_PARITY_HOME\"\ncat \"$CODEX_HOME/config.toml\" > \"$CONTEXT_PARITY_CONFIG\"\nprintf 'context-probe-ok\\n'\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func verifyProfileFiles(root string) error {
	profile := filepath.Join(root, "profile")
	config, err := os.ReadFile(filepath.Join(profile, "config.toml"))
	if err != nil || string(config) != probeProfileConfig {
		return fmt.Errorf("profile config changed under %s", root)
	}
	cache, err := os.ReadFile(filepath.Join(profile, "models_cache.json"))
	if err != nil || string(cache) != `{"models":[{"slug":"gpt-6.1-sol","context_window":272000,"max_context_window":872000}]}` {
		return fmt.Errorf("model cache changed under %s", root)
	}
	return nil
}

func runCommand(parent context.Context, workingDir, binary string, environment []string, arguments ...string) error {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = environment
	command.Dir = workingDir
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("exit: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parseChildArgs(arguments []string) childArgs {
	var result childArgs
	for index := 0; index < len(arguments); index++ {
		switch {
		case arguments[index] == "--model" && index+1 < len(arguments):
			result.Model = arguments[index+1]
		case strings.HasPrefix(arguments[index], "--model="):
			result.Model = strings.TrimPrefix(arguments[index], "--model=")
		case arguments[index] == "-c" && index+1 < len(arguments):
			parseContextOverride(&result, arguments[index+1])
		case strings.HasPrefix(arguments[index], "-c"):
			parseContextOverride(&result, strings.TrimPrefix(arguments[index], "-c"))
		}
	}
	return result
}

func parseContextOverride(result *childArgs, value string) {
	key, raw, found := strings.Cut(strings.TrimSpace(value), "=")
	if !found {
		return
	}
	switch strings.TrimSpace(key) {
	case "model_context_window":
		result.ContextWindow = strings.TrimSpace(raw)
	case "model_auto_compact_token_limit":
		result.CompactLimit = strings.TrimSpace(raw)
	}
}

func validateProbe(got, want childArgs) error {
	if got.Model != want.Model || got.ContextWindow != want.ContextWindow || got.CompactLimit != want.CompactLimit {
		return fmt.Errorf("child context = %#v, want %#v", got, want)
	}
	return nil
}

func checkNegativeControl() error {
	item := probe{Name: "negative-control", Model: "gpt-6.1-sol", ContextWindow: "500000", CompactLimit: "430000"}
	mutated := childArgs{Model: item.Model, ContextWindow: "872000", CompactLimit: "784800"}
	if validateProbe(mutated, childArgs{Model: item.Model, ContextWindow: item.ContextWindow, CompactLimit: item.CompactLimit}) == nil {
		return errors.New("negative control failed: cached maximum promotion was accepted")
	}
	return nil
}
