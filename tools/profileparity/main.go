// profileparity validates managed-profile lifecycle behavior against the
// exact Prodex 0.437.1 binary without touching user accounts or credentials.
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
	"runtime/debug"
	"strings"
	"time"
)

const canonicalProdexCommit = "98918c32e0398990fccf45901d94da0c545d0810"
const canonicalProdexBinarySHA256 = "171482e7ce38ebfd04b5efa564d3d118c7f542b27f30065fa29dd737d18d9d88"

type cliOptions struct {
	prodex       string
	godex        string
	prodexSource string
	godexSource  string
	godexCommit  string
}
type resultReport struct {
	Status          string       `json:"status"`
	CanonicalSource string       `json:"canonical_source"`
	CandidateSource string       `json:"candidate_source"`
	Steps           []stepResult `json:"steps"`
}
type stepResult struct {
	Name       string   `json:"name"`
	Active     string   `json:"active"`
	Profiles   []string `json:"profiles"`
	ProdexExit int      `json:"prodex_exit"`
	GodexExit  int      `json:"godex_exit"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	var options cliOptions
	flags := flag.NewFlagSet("profileparity", flag.ContinueOnError)
	flags.StringVar(&options.prodex, "prodex", "", "official Prodex 0.437.1 binary")
	flags.StringVar(&options.godex, "godex", "", "Godex candidate built from a clean commit")
	flags.StringVar(&options.prodexSource, "prodex-source", "", "exact tagged Prodex source")
	flags.StringVar(&options.godexSource, "godex-source", "", "exact committed Godex source")
	flags.StringVar(&options.godexCommit, "godex-commit", "", "expected full Godex commit SHA")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if options.prodex == "" || options.godex == "" || options.prodexSource == "" ||
		options.godexSource == "" || len(options.godexCommit) != 40 {
		return errors.New("usage: profileparity --prodex BIN --godex BIN --prodex-source DIR --godex-source DIR --godex-commit SHA")
	}
	if err := verifyGitSource(options.prodexSource, canonicalProdexCommit); err != nil {
		return fmt.Errorf("invalid exact Prodex oracle: %w", err)
	}
	if err := verifyGitSource(options.godexSource, options.godexCommit); err != nil {
		return fmt.Errorf("invalid Godex candidate source: %w", err)
	}
	info, err := buildinfo.ReadFile(options.godex)
	if err != nil {
		return fmt.Errorf("Godex build metadata: %w", err)
	}
	if err := verifyBuildMetadata(info.Settings, options.godexCommit); err != nil {
		return err
	}
	digest, err := sha256File(options.prodex)
	if err != nil {
		return fmt.Errorf("read Prodex artifact: %w", err)
	}
	if digest != canonicalProdexBinarySHA256 {
		return fmt.Errorf("Prodex binary digest mismatch: %s", digest)
	}
	output, err := exec.Command(options.prodex, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "prodex 0.437.1" {
		return fmt.Errorf("unexpected Prodex binary version: %q: %v", output, err)
	}
	work, err := os.MkdirTemp("", "godex-profile-parity-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	stages, err := checkProfileLifecycle(work, options)
	if err != nil {
		return fmt.Errorf("profile parity failed closed: %w", err)
	}
	encoded, err := json.MarshalIndent(resultReport{
		Status: "PASS", CanonicalSource: canonicalProdexCommit,
		CandidateSource: options.godexCommit, Steps: stages,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
func verifyBuildMetadata(settings []debug.BuildSetting, expected string) error {
	revision, modified := "", ""
	for _, setting := range settings {
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
	cmd := exec.CommandContext(ctx, "git", "-C", full, "rev-parse", "HEAD")
	raw, err := cmd.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) != expected {
		return fmt.Errorf("source HEAD does not match expected %s", expected)
	}
	raw, err = exec.CommandContext(ctx, "git", "-C", full, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) != 0 {
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
