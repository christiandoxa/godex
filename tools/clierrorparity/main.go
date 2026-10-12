// clierrorparity checks CLI argument rejection against the exact Prodex
// 0.437.7 executable without touching user homes or credentials.
package main

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	canonicalProdexCommit       = "7c774609cf9920d509539cf0e248872f53f6396e"
	canonicalProdexBinarySHA256 = "1159ad3476e6e2ae1fd66cf407da23dfd1da0cdf02eb16c326dbce83440d2c13"
)

type cliOptions struct {
	prodex       string
	godex        string
	prodexSource string
	godexSource  string
	godexCommit  string
}

type probe struct {
	Name      string
	Arguments []string
	Rejects   bool
}

type commandResult struct {
	ExitCode int
	Accepted bool
	Output   string
}

type probeResult struct {
	Name          string   `json:"name"`
	Arguments     []string `json:"arguments"`
	ProdexExit    int      `json:"prodex_exit"`
	GodexExit     int      `json:"godex_exit"`
	ProdexAccepts bool     `json:"prodex_accepts"`
	GodexAccepts  bool     `json:"godex_accepts"`
}

type report struct {
	Status          string        `json:"status"`
	CanonicalSource string        `json:"canonical_source"`
	CandidateSource string        `json:"candidate_source"`
	NegativeControl string        `json:"negative_control"`
	Probes          []probeResult `json:"probes"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	options, err := parseOptions()
	if err != nil {
		return err
	}
	if err := verifyGitSource(options.prodexSource, canonicalProdexCommit); err != nil {
		return fmt.Errorf("invalid exact Prodex oracle: %w", err)
	}
	if err := verifyGitSource(options.godexSource, options.godexCommit); err != nil {
		return fmt.Errorf("invalid Godex candidate source: %w", err)
	}
	if err := verifyProdexBinary(options.prodex); err != nil {
		return err
	}
	if err := verifyGodexBinary(options.godex, options.godexCommit); err != nil {
		return err
	}

	results := make([]probeResult, 0, len(cliProbes()))
	for _, item := range cliProbes() {
		prodex, err := runCommand(options.prodex, "prodex", item.Arguments)
		if err != nil {
			return fmt.Errorf("%s Prodex: %w", item.Name, err)
		}
		godex, err := runCommand(options.godex, "godex", item.Arguments)
		if err != nil {
			return fmt.Errorf("%s Godex: %w", item.Name, err)
		}
		if differences := compareProbe(item, prodex, godex); len(differences) != 0 {
			return fmt.Errorf("%s parity mismatch: %s", item.Name, strings.Join(differences, "; "))
		}
		results = append(results, probeResult{
			Name: item.Name, Arguments: append([]string(nil), item.Arguments...),
			ProdexExit: prodex.ExitCode, GodexExit: godex.ExitCode,
			ProdexAccepts: prodex.Accepted, GodexAccepts: godex.Accepted,
		})
	}
	if err := checkNegativeControl(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report{
		Status: "PASS", CanonicalSource: canonicalProdexCommit,
		CandidateSource: options.godexCommit,
		NegativeControl: "PASS: accepted invalid argument is detected",
		Probes:          results,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func parseOptions() (cliOptions, error) {
	var options cliOptions
	flags := flag.NewFlagSet("clierrorparity", flag.ContinueOnError)
	flags.StringVar(&options.prodex, "prodex", "", "official Prodex 0.437.7 binary")
	flags.StringVar(&options.godex, "godex", "", "Godex candidate binary")
	flags.StringVar(&options.prodexSource, "prodex-source", "", "exact tagged Prodex source")
	flags.StringVar(&options.godexSource, "godex-source", "", "exact committed Godex source")
	flags.StringVar(&options.godexCommit, "godex-commit", "", "expected full Godex source SHA")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return options, err
	}
	if options.prodex == "" || options.godex == "" || options.prodexSource == "" ||
		options.godexSource == "" || len(options.godexCommit) != 40 {
		return options, errors.New("usage: clierrorparity --prodex BIN --godex BIN --prodex-source DIR --godex-source DIR --godex-commit SHA")
	}
	return options, nil
}

func cliProbes() []probe {
	return []probe{
		{Name: "current_valid", Arguments: []string{"current"}, Rejects: false},
		{Name: "current_positional_rejected", Arguments: []string{"current", "synthetic-extra"}, Rejects: true},
		{Name: "profile_list_positional_rejected", Arguments: []string{"profile", "list", "synthetic-extra"}, Rejects: true},
		{Name: "session_list_positional_rejected", Arguments: []string{"session", "list", "synthetic-extra"}, Rejects: true},
		{Name: "info_positional_rejected", Arguments: []string{"info", "synthetic-extra"}, Rejects: true},
		{Name: "status_unknown_option_rejected", Arguments: []string{"status", "--json"}, Rejects: true},
		{Name: "profile_add_missing_name_rejected", Arguments: []string{"profile", "add"}, Rejects: true},
		{Name: "profile_remove_missing_selector_rejected", Arguments: []string{"profile", "remove"}, Rejects: true},
	}
}

func compareProbe(item probe, prodex, godex commandResult) []string {
	var differences []string
	if prodex.Accepted == item.Rejects {
		differences = append(differences, fmt.Sprintf("canonical acceptance=%t, want=%t", prodex.Accepted, !item.Rejects))
	}
	if godex.Accepted != prodex.Accepted {
		differences = append(differences, fmt.Sprintf("candidate acceptance=%t, canonical=%t", godex.Accepted, prodex.Accepted))
	}
	if item.Rejects && strings.TrimSpace(prodex.Output) == "" {
		differences = append(differences, "canonical rejection had no diagnostic")
	}
	if item.Rejects && strings.TrimSpace(godex.Output) == "" {
		differences = append(differences, "candidate rejection had no diagnostic")
	}
	return differences
}

func checkNegativeControl() error {
	item := probe{Name: "negative-control", Arguments: []string{"current", "synthetic-extra"}, Rejects: true}
	canonical := commandResult{ExitCode: 2, Output: "unexpected argument"}
	mutatedCandidate := commandResult{ExitCode: 0, Accepted: true, Output: "No active profile."}
	if len(compareProbe(item, canonical, mutatedCandidate)) == 0 {
		return errors.New("negative control failed: accepted invalid argument was not detected")
	}
	return nil
}

func runCommand(binary, name string, arguments []string) (commandResult, error) {
	root, err := os.MkdirTemp("", "godex-cli-error-parity-")
	if err != nil {
		return commandResult{}, err
	}
	defer os.RemoveAll(root)
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(root, "home"),
		"PRODEX_HOME=" + filepath.Join(root, "prodex"), "GODEX_HOME=" + filepath.Join(root, "godex"),
		"CODEX_HOME=" + filepath.Join(root, "home", ".codex"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"TMPDIR=" + root, "TEMP=" + root, "TMP=" + root,
		"NO_COLOR=1", "CI=1", "TERM=dumb", "PRODEX_NO_UPDATE_CHECK=1", "GODEX_NO_UPDATE_CHECK=1",
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "ALL_PROXY=http://127.0.0.1:1",
	}
	if value := os.Getenv("SystemRoot"); value != "" {
		env = append(env, "SystemRoot="+value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir, command.Env = root, env
	output, commandErr := command.CombinedOutput()
	if ctx.Err() != nil {
		return commandResult{}, fmt.Errorf("%s deadline: %w", name, ctx.Err())
	}
	if len(output) > 16<<10 {
		return commandResult{}, fmt.Errorf("%s diagnostic exceeded 16 KiB", name)
	}
	result := commandResult{Output: string(output)}
	if commandErr == nil {
		result.Accepted = true
		return result, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(commandErr, &exitErr) {
		return commandResult{}, commandErr
	}
	result.ExitCode = exitErr.ExitCode()
	return result, nil
}

func verifyProdexBinary(path string) error {
	digest, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("read Prodex artifact: %w", err)
	}
	if digest != canonicalProdexBinarySHA256 {
		return fmt.Errorf("Prodex binary digest mismatch: %s", digest)
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "prodex 0.437.7" {
		return fmt.Errorf("unexpected Prodex binary version: %q: %v", output, err)
	}
	return nil
}

func verifyGodexBinary(path, expected string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Godex build metadata: %w", err)
	}
	revision, modified := "", ""
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" {
			modified = setting.Value
		}
	}
	if revision != expected || modified != "false" {
		return fmt.Errorf("candidate executable is stale or dirty: revision=%q dirty=%q", revision, modified)
	}
	return nil
}

func verifyGitSource(root, expected string) error {
	full, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", full, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != expected {
		return fmt.Errorf("source HEAD does not match expected %s", expected)
	}
	output, err = exec.CommandContext(ctx, "git", "-C", full, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(output))) != 0 {
		return errors.New("source is modified or has untracked artifacts")
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
