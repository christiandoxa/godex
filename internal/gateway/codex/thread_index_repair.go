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
	if !filepath.IsAbs(sharedCodexHome) {
		return false
	}
	activePath := filepath.Join(codexHome, "sessions")
	sharedPath := filepath.Join(sharedCodexHome, "sessions")
	activeResolved, activeErr := filepath.EvalSymlinks(activePath)
	if activeErr == nil {
		activePath = activeResolved
	}
	sharedResolved, sharedErr := filepath.EvalSymlinks(sharedPath)
	if sharedErr == nil {
		sharedPath = sharedResolved
	}
	return activePath == sharedPath
}
