package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	recursionMarker     = "GODEX_SUB_AGENT"
	launcherMarker      = "GODEX_SUB_AGENT_LAUNCHER"
	configMaxBytes      = 65_536
	hardMaxConcurrency  = 64
	concurrencyExitCode = 75
	cancelledExitCode   = 130
	outputDrainTimeout  = 5 * time.Second
	childReapTimeout    = 5 * time.Second
)

var optionalToolIDs = map[string]bool{
	"caveman":             true,
	"rtk":                 true,
	"codebase-memory-mcp": true,
	"playwright-mcp":      true,
	"ponytail":            true,
	"presidio":            true,
}

type ExitError struct {
	Code    int
	Message string
}

func (err *ExitError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *ExitError) ExitCode() int {
	if err == nil {
		return -1
	}
	return err.Code
}

type maxConcurrency struct {
	Value  uint16 `json:"value"`
	Source string `json:"source"`
}

type childLaunchSpec struct {
	Executable      string         `json:"executable"`
	Provider        string         `json:"provider"`
	Model           *string        `json:"model"`
	Effort          *string        `json:"effort"`
	LocalURL        *string        `json:"local-url"`
	PresidioEnabled bool           `json:"presidio-enabled"`
	RequiredTools   []string       `json:"required-tools"`
	MaxConcurrency  maxConcurrency `json:"max-concurrency"`
	SlotDir         string         `json:"slot-dir"`
	TaskDir         string         `json:"task-dir"`
	TaskMaxBytes    int            `json:"task-max-bytes"`
	RecursionMarker string         `json:"recursion-marker"`
}

type arguments struct {
	configPath string
	taskFile   string
}

func RunArguments(ctx context.Context, values []string, stdout, stderr io.Writer) error {
	args, err := parseArguments(values)
	if err != nil {
		return err
	}
	return run(ctx, args, stdout, stderr)
}

func parseArguments(values []string) (arguments, error) {
	var args arguments
	for index := 0; index < len(values); {
		switch values[index] {
		case "--config":
			if index+1 >= len(values) || strings.TrimSpace(values[index+1]) == "" {
				return arguments{}, errors.New("--config requires a value")
			}
			args.configPath = values[index+1]
			index += 2
		case "--task-file":
			if index+1 >= len(values) || strings.TrimSpace(values[index+1]) == "" {
				return arguments{}, errors.New("--task-file requires a value")
			}
			args.taskFile = values[index+1]
			index += 2
		default:
			return arguments{}, fmt.Errorf("unknown __sub-agent-exec option %q", values[index])
		}
	}
	if args.configPath == "" || args.taskFile == "" {
		return arguments{}, errors.New("usage: godex __sub-agent-exec --config PATH --task-file PATH")
	}
	return args, nil
}

func run(ctx context.Context, args arguments, stdout, stderr io.Writer) error {
	if _, set := os.LookupEnv(recursionMarker); set && os.Getenv(launcherMarker) != "1" {
		return fmt.Errorf("hidden sub-agent launcher cannot be invoked recursively while %s is set", recursionMarker)
	}
	configBytes, err := readBoundedUTF8(args.configPath, configMaxBytes, "sub-agent launcher config")
	if err != nil {
		return err
	}
	spec, err := decodeSpec(configBytes)
	if err != nil {
		return err
	}
	if err := validateSpec(spec); err != nil {
		return err
	}
	taskDir, err := filepath.EvalSymlinks(spec.TaskDir)
	if err != nil {
		return fmt.Errorf("failed to resolve task directory %s: %w", spec.TaskDir, err)
	}
	taskDir, err = filepath.Abs(taskDir)
	if err != nil {
		return err
	}
	taskPath, err := filepath.EvalSymlinks(args.taskFile)
	if err != nil {
		return fmt.Errorf("failed to resolve task file %s: %w", args.taskFile, err)
	}
	taskPath, err = filepath.Abs(taskPath)
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(taskPath)) != filepath.Clean(taskDir) {
		return errors.New("sub-agent task file must be directly inside the configured task directory")
	}
	if err := os.Chmod(taskPath, 0o600); err != nil {
		return fmt.Errorf("failed to secure sub-agent task file: %w", err)
	}
	taskBytes, err := readBoundedUTF8(taskPath, spec.TaskMaxBytes, "sub-agent task")
	if err != nil {
		return err
	}
	task := string(taskBytes)
	if strings.TrimSpace(task) == "" {
		return errors.New("sub-agent task must be nonempty")
	}
	release, err := acquireSlot(spec)
	if err != nil {
		return err
	}
	defer release()

	return runChild(ctx, spec, task, taskPath, stdout, stderr)
}

func decodeSpec(content []byte) (childLaunchSpec, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var spec childLaunchSpec
	if err := decoder.Decode(&spec); err != nil {
		return childLaunchSpec{}, fmt.Errorf("invalid sub-agent launcher config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return childLaunchSpec{}, errors.New("invalid sub-agent launcher config: trailing JSON")
	}
	return spec, nil
}

func validateSpec(spec childLaunchSpec) error {
	if !filepath.IsAbs(spec.Executable) {
		return errors.New("sub-agent executable path must be absolute")
	}
	if spec.RecursionMarker != recursionMarker {
		return errors.New("sub-agent recursion marker is invalid")
	}
	if spec.TaskMaxBytes < 1 || spec.TaskMaxBytes > 65_536 {
		return errors.New("sub-agent task size policy is invalid")
	}
	if spec.MaxConcurrency.Value < 1 || spec.MaxConcurrency.Value > hardMaxConcurrency {
		return fmt.Errorf("maximum active sub-agents must be between 1 and %d", hardMaxConcurrency)
	}
	switch spec.MaxConcurrency.Source {
	case "default", "preset", "custom":
	default:
		return errors.New("sub-agent max-concurrency source is invalid")
	}
	switch spec.Provider {
	case "openai", "anthropic", "copilot", "deepseek", "gemini", "kiro", "local":
	default:
		return fmt.Errorf("invalid sub-agent provider %q", spec.Provider)
	}
	if spec.Provider == "local" && spec.LocalURL == nil {
		return errors.New("local child provider requires a URL")
	}
	if spec.Provider != "local" && spec.LocalURL != nil {
		return errors.New("child local URL is valid only for the local provider")
	}
	if spec.LocalURL != nil {
		if err := validateCredentialFreeURL(*spec.LocalURL); err != nil {
			return err
		}
	}
	if spec.Model != nil && strings.TrimSpace(*spec.Model) == "" {
		return errors.New("sub-agent model must be nonempty")
	}
	if spec.Effort != nil {
		switch strings.ToLower(strings.TrimSpace(*spec.Effort)) {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		default:
			return fmt.Errorf("invalid sub-agent reasoning effort %q", *spec.Effort)
		}
	}
	for _, tool := range spec.RequiredTools {
		if !optionalToolIDs[tool] {
			return fmt.Errorf("invalid required optional tool %s: unknown optional tool %s", tool, tool)
		}
	}
	if strings.TrimSpace(spec.SlotDir) == "" || strings.TrimSpace(spec.TaskDir) == "" {
		return errors.New("sub-agent slot/task directory is required")
	}
	return nil
}

func validateCredentialFreeURL(value string) error {
	invalid := func() error {
		return errors.New("invalid --sub-agent-url: expected an absolute http(s) URL with host and no credentials, query, or fragment")
	}
	if strings.HasPrefix(value, "http:///") || strings.HasPrefix(value, "https:///") {
		return invalid()
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		strings.Contains(value, "#") {
		return invalid()
	}
	return nil
}

func readBoundedUTF8(path string, maxBytes int, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", label, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", label, err)
	}
	if len(content) > maxBytes {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", label, maxBytes)
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("%s must be valid UTF-8", label)
	}
	return content, nil
}

func acquireSlot(spec childLaunchSpec) (func() error, error) {
	for index := uint16(0); index < spec.MaxConcurrency.Value; index++ {
		path := filepath.Join(spec.SlotDir, fmt.Sprintf("slot-%02d.lock", index))
		release, err := lockfile.TryAcquireExisting(path)
		switch {
		case err == nil:
			return release, nil
		case errors.Is(err, lockfile.ErrBusy):
			continue
		default:
			return nil, fmt.Errorf("failed to acquire sub-agent concurrency slot: %w", err)
		}
	}
	return nil, &ExitError{
		Code:    concurrencyExitCode,
		Message: "sub-agent concurrency limit reached; wait for an active child to finish before retrying",
	}
}

func childArgv(spec childLaunchSpec, task string) []string {
	args := []string{"s", "--no-sub-agent"}
	if spec.PresidioEnabled {
		args = append(args, "--presidio")
	} else {
		args = append(args, "--no-presidio")
	}
	for _, tool := range spec.RequiredTools {
		args = append(args, "--require-tool", tool)
	}
	switch spec.Provider {
	case "openai":
		args = append(args, "-c", "model_provider=\"openai\"")
	case "local":
		args = append(args, "--url", *spec.LocalURL)
	default:
		args = append(args, "--provider", spec.Provider)
	}
	if spec.Model != nil {
		args = append(args, "--model", *spec.Model)
	}
	if spec.Effort != nil {
		args = append(args, "-c", "model_reasoning_effort="+strings.ToLower(strings.TrimSpace(*spec.Effort)))
	}
	return append(args, "exec", task)
}

type relayResult struct {
	bytes uint64
	err   error
}

func runChild(
	ctx context.Context,
	spec childLaunchSpec,
	task, taskPath string,
	stdout, stderr io.Writer,
) error {
	command := exec.Command(spec.Executable, childArgv(spec, task)...)
	command.Env = subAgentChildEnvironment()
	configureProcessGroup(command)
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to spawn sub-agent child: %w", err)
	}
	if err := os.Remove(taskPath); err != nil {
		stopProcessTree(command)
		_ = command.Wait()
		return fmt.Errorf("failed to remove consumed task file: %w", err)
	}

	stdoutResult := make(chan relayResult, 1)
	stderrResult := make(chan relayResult, 1)
	go relayChildOutput(stdoutPipe, stdout, stdoutResult)
	go relayChildOutput(stderrPipe, stderr, stderrResult)

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	cancelled := false
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-ctx.Done():
		cancelled = true
		stopProcessTree(command)
		select {
		case waitErr = <-wait:
		case <-time.After(childReapTimeout):
			return errors.New("timed out while reaping cancelled sub-agent child")
		}
	}
	stopProcessTree(command)

	outputBytes, outputIncomplete := drainRelayResults(stdoutResult, stderrResult)
	if cancelled {
		message := "sub-agent launcher cancelled"
		if outputIncomplete {
			message += "; child output was incomplete"
		}
		return &ExitError{Code: cancelledExitCode, Message: message}
	}
	if waitErr != nil {
		code := childExitCode(waitErr)
		message := fmt.Sprintf("sub-agent child exited with status %d", code)
		if outputIncomplete {
			message += "; child output was incomplete"
		}
		return &ExitError{Code: code, Message: message}
	}
	if outputIncomplete {
		return errors.New("sub-agent child output collection failed")
	}
	if outputBytes == 0 {
		return errors.New("sub-agent child completed without output")
	}
	return nil
}

func relayChildOutput(reader io.Reader, writer io.Writer, result chan<- relayResult) {
	var total uint64
	var writeErr error
	buffer := make([]byte, 8192)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			total += uint64(count)
			if writeErr == nil {
				if _, currentErr := writer.Write(buffer[:count]); currentErr != nil {
					writeErr = currentErr
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) && writeErr == nil {
				writeErr = err
			}
			result <- relayResult{bytes: total, err: writeErr}
			return
		}
	}
}

func drainRelayResults(stdout, stderr <-chan relayResult) (uint64, bool) {
	timer := time.NewTimer(outputDrainTimeout)
	defer timer.Stop()
	var total uint64
	incomplete := false
	for count := 0; count < 2; count++ {
		select {
		case result := <-stdout:
			total += result.bytes
			incomplete = incomplete || result.err != nil
			stdout = nil
		case result := <-stderr:
			total += result.bytes
			incomplete = incomplete || result.err != nil
			stderr = nil
		case <-timer.C:
			return total, true
		}
	}
	return total, incomplete
}

func subAgentChildEnvironment() []string {
	result := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (key == recursionMarker || key == launcherMarker) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, recursionMarker+"=1")
}

func childExitCode(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	return signalExitCode(exitErr)
}
