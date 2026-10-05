package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	threadIndexRepairTimeout = 60 * time.Second
)

// ReconcileThreadIndex asks Codex app-server to scan active and archived threads.
func (process *CodexProcess) ReconcileThreadIndex(ctx context.Context, codexHome, sharedCodexHome string) error {
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	if err := validateCodexHomePath(codexHome); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	runContext, cancel := context.WithTimeout(ctx, threadIndexRepairTimeout)
	defer cancel()
	command := exec.Command(binary, "app-server")
	command.Env = codexThreadIndexEnvironment(codexHome, sharedCodexHome)
	command.Stderr = io.Discard
	configureCodexAppServerProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("capture thread index reconciliation output: %w", err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("capture thread index reconciliation input: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start codex app-server for thread index reconciliation: %w", err)
	}

	protocolDone := make(chan error, 1)
	go func() {
		protocolDone <- reconcileCodexThreadIndexProtocol(stdout, stdin)
	}()

	var protocolErr error
	protocolFinished := false
	select {
	case protocolErr = <-protocolDone:
		protocolFinished = true
	case <-runContext.Done():
		protocolErr = runContext.Err()
	}
	terminateCodexAppServerProcess(command)
	_ = stdin.Close()
	_ = command.Wait()

	if !protocolFinished {
		select {
		case workerErr := <-protocolDone:
			if protocolErr == nil {
				protocolErr = workerErr
			}
		case <-time.After(time.Second):
			return errors.Join(protocolErr, errors.New("codex app-server cleanup timed out"))
		}
	}
	if errors.Is(protocolErr, context.DeadlineExceeded) && ctx.Err() == nil {
		return errors.New("thread index reconciliation timed out")
	}
	return protocolErr
}

func codexThreadIndexEnvironment(codexHome, sharedCodexHome string) []string {
	sharedSessions := codexSessionsShareDirectory(codexHome, sharedCodexHome)
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, "CODEX_HOME") {
			continue
		}
		if found && sharedSessions && strings.EqualFold(key, "CODEX_SQLITE_HOME") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "CODEX_HOME="+codexHome)
	if sharedSessions {
		environment = append(environment, "CODEX_SQLITE_HOME="+sharedCodexHome)
	}
	return environment
}

func codexSessionsShareDirectory(codexHome, sharedCodexHome string) bool {
	activePath := normalizeCodexPathForCompare(filepath.Join(codexHome, "sessions"))
	sharedPath := normalizeCodexPathForCompare(filepath.Join(sharedCodexHome, "sessions"))
	return activePath == sharedPath
}

func normalizeCodexPathForCompare(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	lexical := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(lexical); err == nil {
		return filepath.Clean(resolved)
	}

	cursor := lexical
	missing := make([]string, 0, 4)
	for {
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
		missing = append(missing, filepath.Base(cursor))
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		cursor = parent
	}
	return lexical
}

// RepairSessionIndex runs full managed-session maintenance before asking Codex
// app-server to reconcile the active thread index.
func (process *CodexProcess) RepairSessionIndex(ctx context.Context, codexHome, sharedCodexHome, cacheRoot string) error {
	started := time.Now()
	defer process.emitRuntimeTiming("startup.thread_index_reconcile_ms", started)
	if err := MaintainManagedSessions(sharedCodexHome, cacheRoot); err != nil {
		return fmt.Errorf("full session index repair failed: %w", err)
	}
	if err := process.ReconcileThreadIndex(ctx, codexHome, sharedCodexHome); err != nil {
		return fmt.Errorf("full session index repair failed: %w", err)
	}
	return nil
}

func (process *CodexProcess) emitRuntimeTiming(stage string, started time.Time) {
	if _, enabled := os.LookupEnv("PRODEX_RUNTIME_TIMINGS"); !enabled || process == nil || process.terminal.Stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(
		process.terminal.Stderr,
		"prodex_runtime_timing stage=%s duration_ms=%g\n",
		stage,
		float64(time.Since(started))/float64(time.Millisecond),
	)
}
