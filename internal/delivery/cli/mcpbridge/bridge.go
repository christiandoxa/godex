package mcpbridge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/mcpstdio"
)

const (
	monitorInterval    = 10 * time.Millisecond
	outputDrainTimeout = time.Second
	stderrLimit        = 64 * 1024
)

type framingState struct {
	mu    sync.Mutex
	value mcpstdio.Framing
}

func (state *framingState) set(value mcpstdio.Framing) {
	state.mu.Lock()
	state.value = value
	state.mu.Unlock()
}
func (state *framingState) get() mcpstdio.Framing {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.value
}

type stderrResult struct {
	content   []byte
	truncated bool
	err       error
}

func Run(ctx context.Context, command string, arguments []string, input io.Reader, output io.Writer) error {
	return runWithEnv(ctx, nil, command, arguments, input, output)
}

func runWithEnv(ctx context.Context, extraEnv []string, command string, arguments []string, input io.Reader, output io.Writer) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("MCP server command is required")
	}
	if input == nil || output == nil {
		return errors.New("MCP bridge input and output are required")
	}
	cmd := exec.Command(command, arguments...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	configureProcessGroup(cmd)
	childStdinRead, childIn, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create MCP server stdin pipe: %w", err)
	}
	childOut, childStdoutWrite, err := os.Pipe()
	if err != nil {
		_ = childStdinRead.Close()
		_ = childIn.Close()
		return fmt.Errorf("failed to create MCP server stdout pipe: %w", err)
	}
	childErr, childStderrWrite, err := os.Pipe()
	if err != nil {
		_ = childStdinRead.Close()
		_ = childIn.Close()
		_ = childOut.Close()
		_ = childStdoutWrite.Close()
		return fmt.Errorf("failed to create MCP server stderr pipe: %w", err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = childStdinRead, childStdoutWrite, childStderrWrite
	if err := cmd.Start(); err != nil {
		_ = childStdinRead.Close()
		_ = childIn.Close()
		_ = childOut.Close()
		_ = childStdoutWrite.Close()
		_ = childErr.Close()
		_ = childStderrWrite.Close()
		return fmt.Errorf("failed to start MCP server %s: %w", command, err)
	}
	_ = childStdinRead.Close()
	_ = childStdoutWrite.Close()
	_ = childStderrWrite.Close()

	framing := &framingState{value: mcpstdio.ContentLength}
	outputDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(childOut)
		writer := bufio.NewWriter(output)
		for {
			message, _, err := mcpstdio.ReadMessage(reader)
			if err != nil {
				outputDone <- err
				return
			}
			if message == nil {
				outputDone <- nil
				return
			}
			if err := mcpstdio.WriteMessage(writer, message, framing.get()); err != nil {
				outputDone <- err
				return
			}
		}
	}()

	inputDone := make(chan error, 1)
	go func() {
		defer childIn.Close()
		reader := bufio.NewReader(input)
		writer := bufio.NewWriter(childIn)
		for {
			message, sourceFraming, err := mcpstdio.ReadMessage(reader)
			if err != nil {
				inputDone <- err
				return
			}
			if message == nil {
				inputDone <- nil
				return
			}
			framing.set(sourceFraming)
			if err := mcpstdio.WriteMessage(writer, message, mcpstdio.JSONLine); err != nil {
				inputDone <- err
				return
			}
		}
	}()

	stderrDone := make(chan stderrResult, 1)
	go func() {
		content, truncated, err := readBoundedStderr(childErr)
		stderrDone <- stderrResult{content: content, truncated: truncated, err: err}
	}()
	childDone := make(chan error, 1)
	go func() { childDone <- cmd.Wait() }()

	return monitor(ctx, cmd, outputDone, inputDone, stderrDone, childDone)
}

func monitor(ctx context.Context, cmd *exec.Cmd, outputDone, inputDone <-chan error, stderrDone <-chan stderrResult, childDone <-chan error) error {
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()
	for {
		select {
		case outputErr := <-outputDone:
			if outputErr != nil {
				stopProcessTree(cmd)
				return fmt.Errorf("MCP bridge output: %w", outputErr)
			}
			select {
			case childErr := <-childDone:
				return childStatusResult(childErr, waitStderr(stderrDone))
			case <-time.After(outputDrainTimeout):
			}
			stopProcessTree(cmd)
			awaitChild(childDone)
			return errors.New("MCP server closed stdout before exiting")
		case childErr := <-childDone:
			stopProcessTree(cmd)
			select {
			case outputErr := <-outputDone:
				if outputErr != nil {
					return fmt.Errorf("MCP bridge output: %w", outputErr)
				}
			case <-time.After(outputDrainTimeout):
				return errors.New("MCP server stdout remained open after child exited")
			}
			select {
			case inputErr := <-inputDone:
				if inputErr != nil {
					return fmt.Errorf("MCP bridge input: %w", inputErr)
				}
			default:
			}
			return childStatusResult(childErr, waitStderr(stderrDone))
		case inputErr := <-inputDone:
			var childErr error
			childExited := false
			select {
			case childErr = <-childDone:
				childExited = true
			default:
			}
			stopProcessTree(cmd)
			if !childExited {
				awaitChild(childDone)
			}
			select {
			case outputErr := <-outputDone:
				if outputErr != nil {
					return fmt.Errorf("MCP bridge output: %w", outputErr)
				}
			case <-time.After(outputDrainTimeout):
				return errors.New("MCP bridge output did not drain after input closed")
			}
			stderr := waitStderr(stderrDone)
			if inputErr != nil {
				return fmt.Errorf("MCP bridge input: %w", inputErr)
			}
			if childExited {
				return childStatusResult(childErr, stderr)
			}
			return nil
		case <-ctx.Done():
			stopProcessTree(cmd)
			awaitChild(childDone)
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func readBoundedStderr(reader io.Reader) ([]byte, bool, error) {
	retained := make([]byte, 0, min(stderrLimit, 8*1024))
	buffer := make([]byte, 8*1024)
	truncated := false
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			keep := min(count, stderrLimit-len(retained))
			if keep > 0 {
				retained = append(retained, buffer[:keep]...)
			}
			truncated = truncated || keep < count
		}
		if errors.Is(err, io.EOF) {
			return retained, truncated, nil
		}
		if err != nil {
			return retained, truncated, err
		}
	}
}

func waitStderr(done <-chan stderrResult) stderrResult {
	select {
	case result := <-done:
		return result
	case <-time.After(outputDrainTimeout):
		return stderrResult{err: errors.New("timed out reading MCP server stderr")}
	}
}

func childStatusResult(childErr error, stderr stderrResult) error {
	if stderr.err != nil {
		return fmt.Errorf("failed to read MCP server stderr: %w", stderr.err)
	}
	if childErr == nil {
		return nil
	}
	diagnostic := strings.TrimSpace(string(stderr.content))
	if diagnostic == "" {
		return fmt.Errorf("MCP server exited with status %s", childStatusText(childErr))
	}
	suffix := ""
	if stderr.truncated {
		suffix = " (stderr truncated)"
	}
	return fmt.Errorf("MCP server exited with status %s: %s%s", childStatusText(childErr), diagnostic, suffix)
}

func childStatusText(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ProcessState.String()
	}
	return err.Error()
}

func awaitChild(done <-chan error) {
	select {
	case <-done:
	case <-time.After(outputDrainTimeout):
	}
}

func RunArguments(ctx context.Context, arguments []string, input io.Reader, output io.Writer) error {
	if len(arguments) == 0 || strings.TrimSpace(arguments[0]) == "" {
		return errors.New("__mcp-jsonl-bridge requires COMMAND")
	}
	return Run(ctx, arguments[0], arguments[1:], input, output)
}
