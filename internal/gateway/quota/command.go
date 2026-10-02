package quota

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

const virtualCommandTimeout = 15 * time.Second

type commandResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type commandRunner func(context.Context, string, []string) (commandResult, error)

type commandBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *commandBuffer) Write(content []byte) (int, error) {
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		write := min(len(content), remaining)
		_, _ = buffer.buffer.Write(content[:write])
	}
	if len(content) > remaining {
		buffer.truncated = true
	}
	return len(content), nil
}

func runVirtualCommand(ctx context.Context, binary string, arguments []string) (commandResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, virtualCommandTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, arguments...)
	stdout := &commandBuffer{limit: virtualBodyMaxBytes}
	stderr := &commandBuffer{limit: virtualBodyMaxBytes}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return commandResult{}, ctx.Err()
		}
		return commandResult{}, errors.New("provider quota command timed out")
	}
	if stdout.truncated || stderr.truncated {
		return commandResult{}, errors.New("provider quota command exceeded the output limit")
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
	return commandResult{}, errors.New("failed to execute provider quota command")
}
