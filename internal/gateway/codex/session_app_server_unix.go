//go:build !windows

package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const sessionAppServerStartupTimeout = 60 * time.Second

func (process *CodexProcess) runWithSessionAppServer(
	ctx context.Context,
	binary, codexHome string,
	environment []string,
	command *exec.Cmd,
	arguments []string,
) error {
	socket, cleanup, err := privateSessionAppServerSocket(codexHome)
	if err != nil {
		return err
	}
	defer cleanup()

	companion := exec.Command(binary, sessionAppServerCompanionArguments(arguments, socket)...)
	companion.Dir = codexHome
	companion.Env = append([]string(nil), environment...)
	companion.Stdin = nil
	companion.Stdout = io.Discard
	companion.Stderr = io.Discard
	configureCodexAppServerProcess(companion)
	if err := companion.Start(); err != nil {
		return fmt.Errorf("start Codex app-server companion: %w", err)
	}
	companionDone := make(chan struct{})
	go func() {
		_ = companion.Wait()
		close(companionDone)
	}()
	var cleanOnce sync.Once
	cleanCompanion := func() {
		cleanOnce.Do(func() {
			select {
			case <-companionDone:
				return
			default:
				terminateCodexAppServerProcess(companion)
			}
			<-companionDone
		})
	}
	defer cleanCompanion()
	if err := waitForSessionAppServerSocket(ctx, socket, companionDone); err != nil {
		return err
	}

	command.Args = append([]string{command.Path, "--remote", "unix://" + socket}, arguments...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Codex child with app-server companion: %w", err)
	}
	if err := waitForSessionChild(ctx, command); err != nil {
		return err
	}
	return nil
}

// privateSessionAppServerSocket keeps the usual profile-owned socket when
// its path is portable to macOS, and uses an exclusive 0700 temporary
// directory for longer paths (which Unix sockets cannot bind reliably).
func privateSessionAppServerSocket(codexHome string) (string, func(), error) {
	const portableUnixSocketPathBytes = 90
	socket := filepath.Join(codexHome, ".s")
	if len(socket) <= portableUnixSocketPathBytes {
		if err := prepareSessionAppServerSocket(socket); err != nil {
			return "", nil, err
		}
		return socket, func() { _ = os.Remove(socket) }, nil
	}
	directory, err := os.MkdirTemp("", "gd-session-")
	if err != nil {
		return "", nil, fmt.Errorf("create private Codex app-server socket directory: %w", err)
	}
	socket = filepath.Join(directory, ".s")
	if len(socket) > portableUnixSocketPathBytes {
		_ = os.RemoveAll(directory)
		return "", nil, errors.New("temporary Codex app-server socket path is too long")
	}
	if err := prepareSessionAppServerSocket(socket); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, err
	}
	return socket, func() { _ = os.RemoveAll(directory) }, nil
}

func waitForSessionChild(ctx context.Context, command *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		if command.Process != nil {
			_ = command.Process.Signal(os.Interrupt)
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
			return ctx.Err()
		case <-timer.C:
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-done
			return ctx.Err()
		}
	}
}

func prepareSessionAppServerSocket(path string) error {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("inspect Codex app-server socket: %w", err)
	case info.Mode()&os.ModeSymlink != 0:
		return errors.New("Codex app-server socket path must not be a symbolic link")
	case info.Mode()&os.ModeSocket == 0:
		return errors.New("Codex app-server socket path is not a socket")
	default:
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale Codex app-server socket: %w", err)
		}
		return nil
	}
}

func waitForSessionAppServerSocket(ctx context.Context, path string, companionDone <-chan struct{}) error {
	deadline := time.NewTimer(sessionAppServerStartupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		connection, err := net.DialTimeout("unix", path, 250*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		select {
		case <-companionDone:
			return errors.New("Codex app-server companion exited before becoming ready")
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Codex app-server companion did not become ready")
		case <-ticker.C:
		}
	}
}
