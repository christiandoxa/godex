package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type caseResult struct {
	Name   string `json:"name"`
	Prodex string `json:"prodex"`
	Godex  string `json:"godex"`
}

type report struct {
	Status          string       `json:"status"`
	Oracle          string       `json:"oracle"`
	OracleSource    string       `json:"oracle_source"`
	CandidateSource string       `json:"candidate_source"`
	Cases           []caseResult `json:"cases"`
}

func main() {
	ignoreHangup()
	if os.Getenv("SESSION_FOOTER_FAKE") == "1" &&
		(len(os.Args) == 1 || os.Args[1] != "__runtime-goal-session-notify") {
		os.Exit(runFakeCodex())
	}
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	options, err := parseOptions()
	if err != nil {
		return err
	}
	if err := verifyOracle(options.prodex); err != nil {
		return err
	}
	if err := verifySource(options.prodexSource, oracleCommit); err != nil {
		return fmt.Errorf("invalid Prodex oracle source: %w", err)
	}
	if err := verifySource(options.godexSource, options.godexCommit); err != nil {
		return fmt.Errorf("invalid Godex candidate source: %w", err)
	}
	if err := verifyGodexBuild(options.godex, options.godexCommit); err != nil {
		return err
	}
	// Keep the fixture root short: Unix-domain socket paths have a small
	// platform limit, and both launchers append private overlay directories.
	root, err := os.MkdirTemp("/tmp", "gf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	fake, err := writeFakeWrapper(root)
	if err != nil {
		return err
	}
	cases := []struct {
		name  string
		tty   bool
		extra []string
		check func(string, int, string) error
	}{
		{name: "canonical_identity", tty: true, check: checkCanonicalFooter},
		{name: "unknown_identity_picker", tty: true, extra: []string{"SESSION_FOOTER_EMIT_ID=0"}, check: checkUnknownFooter},
		{name: "piped_output_suppressed", tty: false, check: checkNoFooter},
	}
	results := make([]caseResult, 0, len(cases))
	for _, item := range cases {
		prodexOutput, prodexCode, err := launchCase(root, "prodex", options.prodex, fake, item)
		if err != nil {
			return fmt.Errorf("%s Prodex launch: %w", item.name, err)
		}
		if err := item.check(prodexOutput, prodexCode, "prodex"); err != nil {
			return fmt.Errorf("%s Prodex: %w", item.name, err)
		}
		godexOutput, godexCode, err := launchCase(root, "godex", options.godex, fake, item)
		if err != nil {
			return fmt.Errorf("%s Godex launch: %w", item.name, err)
		}
		if err := item.check(godexOutput, godexCode, "godex"); err != nil {
			return fmt.Errorf("%s Godex: %w", item.name, err)
		}
		if footerShape(prodexOutput, "prodex") != footerShape(godexOutput, "godex") {
			return fmt.Errorf("%s footer shape differs between binaries", item.name)
		}
		results = append(results, caseResult{Name: item.name, Prodex: summarize(prodexOutput), Godex: summarize(godexOutput)})
	}
	encoded, err := json.MarshalIndent(report{
		Status: "PASS", Oracle: oracleVersion, OracleSource: oracleCommit,
		CandidateSource: options.godexCommit, Cases: results,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func launchCase(root, name, binary, fake string, item struct {
	name  string
	tty   bool
	extra []string
	check func(string, int, string) error
}) (string, int, error) {
	caseRoot := filepath.Join(root, name, item.name)
	if err := os.MkdirAll(caseRoot, 0o700); err != nil {
		return "", 0, err
	}
	home := filepath.Join(caseRoot, "home")
	godexHome := filepath.Join(caseRoot, "godex")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(godexHome, 0o700); err != nil {
		return "", 0, err
	}
	environment := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"TERM=xterm",
		"NO_COLOR=1",
		"SESSION_FOOTER_ID=" + fakeSessionID,
		"PRODEX_CODEX_BIN=" + fake,
		"GODEX_CODEX_BIN=" + fake,
		"GODEX_HOME=" + godexHome,
	}
	environment = append(environment, item.extra...)
	if err := setupProfile(binary, environment, name); err != nil {
		return "", 0, err
	}
	arguments := []string{
		"super", "--skip-quota-check", "--no-auto-rotate", "--no-presidio", "--no-sub-agent",
		"--provider", "deepseek", "--api-key", "synthetic", "--model", "synthetic", "--profile", "main",
	}
	ctx, cancel := commandContext()
	defer cancel()
	result, err := runCommand(ctx, binary, arguments, environment, item.tty)
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return string(result.output), result.exitCode, err
		}
	}
	return string(result.output), result.exitCode, nil
}

func setupProfile(binary string, environment []string, name string) error {
	ctx, cancel := commandContext()
	defer cancel()
	command := exec.CommandContext(ctx, binary, "profile", "add", "main", "--activate")
	command.Env = environment
	command.Stdin = strings.NewReader("")
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s profile setup: %w", name, err)
	}
	return nil
}

func writeFakeWrapper(root string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "codex-fixture")
	script := "#!/bin/sh\nexport SESSION_FOOTER_FAKE=1\nexec " + shellQuote(executable) + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func checkCanonicalFooter(output string, code int, provider string) error {
	if code != 0 {
		return fmt.Errorf("exit status %d output=%q", code, diagnosticOutput(output))
	}
	footer := normalizedFooter(output, provider)
	if !strings.Contains(footer, "Session ID: "+fakeSessionID+"\n") ||
		!strings.Contains(footer, "Resume: "+provider+" s "+fakeSessionID+"\n") {
		return fmt.Errorf("canonical session identity or resume command missing output=%q", diagnosticOutput(output))
	}
	if !strings.Contains(output, "Native usage summary") ||
		strings.Index(output, "Native usage summary") > strings.Index(output, providerTitle(provider)) {
		return errors.New("footer did not append after native output")
	}
	if strings.Contains(footer, "unix://") || strings.Contains(footer, "\x1b") {
		return errors.New("footer contains socket or terminal control data")
	}
	return nil
}

func checkUnknownFooter(output string, code int, provider string) error {
	if code != 0 {
		return fmt.Errorf("exit status %d output=%q", code, diagnosticOutput(output))
	}
	footer := normalizedFooter(output, provider)
	if !strings.Contains(footer, "Session ID was not reported by this launch.\n") ||
		!strings.Contains(footer, "Resume: "+provider+" s resume\n") {
		return fmt.Errorf("unknown identity did not use picker fallback output=%q", diagnosticOutput(output))
	}
	if strings.Contains(footer, "Session ID: ") {
		return errors.New("unknown identity was guessed")
	}
	return nil
}

func checkNoFooter(output string, code int, provider string) error {
	if code != 0 {
		return fmt.Errorf("exit status %d output=%q", code, diagnosticOutput(output))
	}
	if strings.Contains(strings.ToLower(output), strings.ToLower(providerTitle(provider))) {
		return errors.New("non-TTY output emitted saved-session footer")
	}
	return nil
}

func diagnosticOutput(output string) string {
	if len(output) > 2000 {
		output = output[len(output)-2000:]
	}
	return output
}

func providerTitle(provider string) string {
	if provider == "prodex" {
		return "Prodex saved session"
	}
	return "Godex saved session"
}

func normalizedFooter(output, provider string) string {
	index := strings.Index(output, providerTitle(provider))
	if index < 0 {
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(output[index:], "\r\r\n", "\r\n"), "\r\n", "\n")
}

func footerShape(output, provider string) string {
	footer := normalizedFooter(output, provider)
	return fmt.Sprintf("id=%t picker=%t socket=%t controls=%t",
		strings.Contains(footer, "Session ID: "+fakeSessionID+"\n"),
		strings.Contains(footer, "Resume: "+provider+" s resume\n"),
		strings.Contains(footer, "unix://"),
		strings.Contains(footer, "\x1b"),
	)
}

func summarize(output string) string {
	return fmt.Sprintf("bytes=%d footer=%t", len(output), strings.Contains(output, "saved session"))
}
