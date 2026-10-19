package main

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
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
	oracleVersion = "prodex 0.438.3"
	oracleCommit  = "42042d765100fc06895a27e0904ffb64842092e6"
	oracleSHA256  = "5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923"
)

type options struct {
	prodex       string
	godex        string
	prodexSource string
	godexSource  string
	godexCommit  string
}

func parseOptions() (options, error) {
	var result options
	flags := flag.NewFlagSet("sessionfooterparity", flag.ContinueOnError)
	flags.StringVar(&result.prodex, "prodex", "", "official Prodex 0.438.3 binary")
	flags.StringVar(&result.godex, "godex", "", "Godex candidate binary")
	flags.StringVar(&result.prodexSource, "prodex-source", "", "clean tagged Prodex source")
	flags.StringVar(&result.godexSource, "godex-source", "", "clean Godex source")
	flags.StringVar(&result.godexCommit, "godex-commit", "", "full Godex source commit")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return result, err
	}
	if result.prodex == "" || result.godex == "" || result.prodexSource == "" ||
		result.godexSource == "" || len(result.godexCommit) != 40 {
		return result, errors.New("usage: sessionfooterparity --prodex BIN --godex BIN --prodex-source DIR --godex-source DIR --godex-commit SHA")
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

func verifySource(root, expected string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", absolute, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != expected {
		return fmt.Errorf("source HEAD does not match %s", expected)
	}
	output, err = exec.CommandContext(ctx, "git", "-C", absolute, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "" {
		return errors.New("source is modified or has untracked artifacts")
	}
	return nil
}

func verifyGodexBuild(path string, expected string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Godex build metadata: %w", err)
	}
	revision, modified := "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	if revision != expected || modified != "false" {
		return fmt.Errorf("candidate executable is stale or dirty: revision=%q dirty=%q", revision, modified)
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
