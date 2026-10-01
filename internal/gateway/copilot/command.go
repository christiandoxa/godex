package copilot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	credentialCommandTimeout = 15 * time.Second
	credentialOutputMaxBytes = 1 << 20
)

type commandResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type commandRunner func(context.Context, string, []string) (commandResult, error)

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *boundedBuffer) Write(content []byte) (int, error) {
	available := buffer.limit - buffer.buffer.Len()
	if available > 0 {
		write := len(content)
		if write > available {
			write = available
		}
		_, _ = buffer.buffer.Write(content[:write])
	}
	if len(content) > available {
		buffer.truncated = true
	}
	return len(content), nil
}

func runCredentialCommand(ctx context.Context, binary string, arguments []string) (commandResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, credentialCommandTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, arguments...)
	stdout := &boundedBuffer{limit: credentialOutputMaxBytes}
	stderr := &boundedBuffer{limit: credentialOutputMaxBytes}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return commandResult{}, ctx.Err()
		}
		return commandResult{}, errors.New("Copilot credential command timed out")
	}
	if stdout.truncated || stderr.truncated {
		return commandResult{}, errors.New("Copilot credential command exceeded the output limit")
	}
	result := commandResult{stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return commandResult{}, fmt.Errorf("run Copilot credential command: %w", err)
}
