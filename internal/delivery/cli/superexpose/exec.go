package superexpose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/redact"
)

const (
	execDefaultTimeoutMS  = 30_000
	execMaxTimeoutMS      = 120_000
	execMaxProgramBytes   = 4 * 1024
	execMaxArguments      = 256
	execMaxArgumentBytes  = 16 * 1024
	execMaxArgumentsBytes = 256 * 1024
	execMaxCWDBytes       = 4 * 1024
	execMaxEnvEntries     = 128
	execMaxEnvKeyBytes    = 256
	execMaxEnvValueBytes  = 16 * 1024
	execMaxStdinBytes     = 256 * 1024
	execMaxOutputBytes    = 128 * 1024
)

type execRequest struct {
	requestedProgram string
	program          string
	args             []string
	cwd              string
	env              map[string]string
	stdin            []byte
	timeout          time.Duration
	optionalTool     string
}

type capturedOutput struct {
	bytes     []byte
	truncated bool
}

func executeDirect(ctx context.Context, arguments map[string]any, defaultCWD string, tools optionalToolSnapshot) (map[string]any, error) {
	request, err := parseExecRequest(arguments, defaultCWD, tools)
	if err != nil {
		return nil, err
	}
	return runExec(ctx, request, tools)
}

func parseExecRequest(arguments map[string]any, defaultCWD string, tools optionalToolSnapshot) (execRequest, error) {
	program, ok := arguments["program"].(string)
	if !ok || program == "" {
		return execRequest{}, errors.New("program is required")
	}
	if err := validatePathText(program, "program", execMaxProgramBytes); err != nil {
		return execRequest{}, err
	}
	args, err := parseExecArgs(arguments["args"])
	if err != nil {
		return execRequest{}, err
	}
	resolvedProgram, resolvedArgs, optionalTool, err := tools.resolveProgram(program, args)
	if err != nil {
		return execRequest{}, err
	}
	cwd, err := parseExecCWD(arguments["cwd"], defaultCWD)
	if err != nil {
		return execRequest{}, err
	}
	environment, err := parseExecEnv(arguments["env"])
	if err != nil {
		return execRequest{}, err
	}
	stdin, err := parseExecStdin(arguments["stdin"])
	if err != nil {
		return execRequest{}, err
	}
	timeout, err := parseExecTimeout(arguments["timeout_ms"])
	if err != nil {
		return execRequest{}, err
	}
	return execRequest{
		requestedProgram: program,
		program:          resolvedProgram,
		args:             resolvedArgs,
		cwd:              cwd,
		env:              environment,
		stdin:            stdin,
		timeout:          timeout,
		optionalTool:     optionalTool,
	}, nil
}

func parseExecArgs(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("args must be an array")
	}
	if len(values) > execMaxArguments {
		return nil, fmt.Errorf("args must contain at most %d items", execMaxArguments)
	}
	result := make([]string, 0, len(values))
	total := 0
	for _, raw := range values {
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("args must contain only strings")
		}
		if len(value) > execMaxArgumentBytes || strings.IndexByte(value, 0) >= 0 {
			return nil, errors.New("argument contains NUL or is too large")
		}
		total += len(value)
		if total > execMaxArgumentsBytes {
			return nil, fmt.Errorf("total argument bytes must be at most %d", execMaxArgumentsBytes)
		}
		result = append(result, value)
	}
	return result, nil
}

func parseExecCWD(value any, defaultCWD string) (string, error) {
	if value == nil {
		return defaultCWD, nil
	}
	cwd, ok := value.(string)
	if !ok {
		return "", errors.New("cwd must be a string")
	}
	if err := validatePathText(cwd, "cwd", execMaxCWDBytes); err != nil {
		return "", err
	}
	return cwd, nil
}

func parseExecEnv(value any) (map[string]string, error) {
	result := make(map[string]string)
	if value == nil {
		return result, nil
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("env must be an object")
	}
	if len(values) > execMaxEnvEntries {
		return nil, fmt.Errorf("env must contain at most %d entries", execMaxEnvEntries)
	}
	for key, raw := range values {
		if err := validateEnvKey(key); err != nil {
			return nil, err
		}
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("env values must be strings")
		}
		if len(value) > execMaxEnvValueBytes || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("env value for %s is too large or contains NUL", key)
		}
		result[key] = value
	}
	return result, nil
}

func parseExecStdin(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, errors.New("stdin must be a string")
	}
	if len(text) > execMaxStdinBytes {
		return nil, fmt.Errorf("stdin must be at most %d bytes", execMaxStdinBytes)
	}
	return []byte(text), nil
}

func parseExecTimeout(value any) (time.Duration, error) {
	timeout := uint64(execDefaultTimeoutMS)
	if value != nil {
		switch typed := value.(type) {
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil || parsed < 0 {
				return 0, errors.New("timeout_ms must be a nonnegative integer")
			}
			timeout = uint64(parsed)
		case float64:
			if typed < 0 || typed != float64(uint64(typed)) {
				return 0, errors.New("timeout_ms must be a nonnegative integer")
			}
			timeout = uint64(typed)
		default:
			return 0, errors.New("timeout_ms must be a nonnegative integer")
		}
	}
	if timeout < 1 || timeout > execMaxTimeoutMS {
		return 0, fmt.Errorf("timeout_ms must be between 1 and %d", execMaxTimeoutMS)
	}
	return time.Duration(timeout) * time.Millisecond, nil
}

func validatePathText(value, name string, maxBytes int) error {
	if value == "" || len(value) > maxBytes || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s is empty, contains control characters, or is too large", name)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("%s is empty, contains control characters, or is too large", name)
		}
	}
	return nil
}

func validateEnvKey(key string) error {
	if key == "" || len(key) > execMaxEnvKeyBytes || strings.Contains(key, "=") || strings.IndexByte(key, 0) >= 0 {
		return errors.New("env key is empty, contains '=', control characters, or is too large")
	}
	for _, character := range key {
		if character < 0x20 || character == 0x7f {
			return errors.New("env key is empty, contains '=', control characters, or is too large")
		}
	}
	return nil
}

func runExec(ctx context.Context, request execRequest, tools optionalToolSnapshot) (map[string]any, error) {
	command := exec.Command(request.program, request.args...)
	command.Dir = request.cwd
	configureExecProcess(command)
	environment := inheritedEnvironment()
	for key, value := range request.env {
		environment[key] = value
	}
	delete(environment, "CONTROL_PLANE_API_KEY")
	tools.applyEnvironment(environment)
	delete(environment, "CONTROL_PLANE_API_KEY")
	command.Env = flattenEnvironment(environment)

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, errors.New("failed to capture executable stdin")
	}
	stdoutCapture := newBoundedExecWriter()
	stderrCapture := newBoundedExecWriter()
	command.Stdout = stdoutCapture
	command.Stderr = stderrCapture
	started := time.Now()
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("failed to start executable: %s", redact.Secrets(err.Error()))
	}
	pid := command.Process.Pid
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		_, _ = stdin.Write(request.stdin)
		_ = stdin.Close()
	}()
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	timer := time.NewTimer(request.timeout)
	defer timer.Stop()
	termination := ""
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-timer.C:
		termination = "timeout"
		stopExecProcessTree(command)
		select {
		case waitErr = <-wait:
		case <-time.After(5 * time.Second):
			return nil, errors.New("timed out while reaping terminated executable")
		}
	case <-ctx.Done():
		termination = "cancelled"
		stopExecProcessTree(command)
		select {
		case waitErr = <-wait:
		case <-time.After(5 * time.Second):
			return nil, errors.New("timed out while reaping cancelled executable")
		}
	}
	writerDone := make(chan struct{})
	go func() {
		writer.Wait()
		close(writerDone)
	}()
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		return nil, errors.New("exec stdin writer did not stop")
	}
	stdoutText, stdoutClipped := renderExecOutput(stdoutCapture.snapshot())
	stderrText, stderrClipped := renderExecOutput(stderrCapture.snapshot())

	exitCode, exitStatus, signal := execExitDetails(waitErr)
	status := "completed"
	if termination == "timeout" {
		status = "timed_out"
	} else if termination == "cancelled" {
		status = "cancelled"
	}
	result := map[string]any{
		"status":                   status,
		"program":                  request.requestedProgram,
		"optional_tool":            nil,
		"available_optional_tools": tools.availableIDs(),
		"arg_count":                len(request.args),
		"cwd":                      request.cwd,
		"pid":                      pid,
		"exit_code":                exitCode,
		"exit_status":              exitStatus,
		"signal":                   signal,
		"termination":              nil,
		"success":                  termination == "" && waitErr == nil,
		"duration_ms":              uint64(time.Since(started) / time.Millisecond),
		"stdout":                   stdoutText,
		"stdout_truncated":         stdoutClipped,
		"stderr":                   stderrText,
		"stderr_truncated":         stderrClipped,
	}
	if request.optionalTool != "" {
		result["optional_tool"] = request.optionalTool
	}
	if termination != "" {
		result["termination"] = termination
	}
	return result, nil
}

type boundedExecWriter struct {
	mu     sync.Mutex
	output capturedOutput
}

func newBoundedExecWriter() *boundedExecWriter {
	return &boundedExecWriter{output: capturedOutput{bytes: make([]byte, 0, 8192)}}
}

func (writer *boundedExecWriter) Write(content []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.output.bytes) < execMaxOutputBytes {
		remaining := execMaxOutputBytes - len(writer.output.bytes)
		kept := min(remaining, len(content))
		writer.output.bytes = append(writer.output.bytes, content[:kept]...)
		if kept < len(content) {
			writer.output.truncated = true
		}
	} else if len(content) > 0 {
		writer.output.truncated = true
	}
	return len(content), nil
}

func (writer *boundedExecWriter) snapshot() capturedOutput {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return capturedOutput{bytes: append([]byte(nil), writer.output.bytes...), truncated: writer.output.truncated}
}

func renderExecOutput(output capturedOutput) (string, bool) {
	text := redact.Secrets(string(output.bytes))
	truncated := output.truncated
	if len(text) > execMaxOutputBytes {
		text = truncateUTF8(text, execMaxOutputBytes)
		truncated = true
	}
	return text, truncated
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && end < len(value) && (value[end]&0xc0) == 0x80 {
		end--
	}
	return value[:end]
}

func inheritedEnvironment() map[string]string {
	result := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func flattenEnvironment(environment map[string]string) []string {
	result := make([]string, 0, len(environment))
	for key, value := range environment {
		result = append(result, key+"="+value)
	}
	return result
}

func execExitDetails(waitErr error) (any, int, any) {
	if waitErr == nil {
		return 0, 0, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return nil, 1, nil
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code, code, nil
	}
	signal, status := execExitSignal(exitErr)
	return nil, status, signal
}
