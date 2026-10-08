package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

const codexFileCredentialConfig = "cli_auth_credentials_store = \"file\"\n"

type CodexProcess struct {
	binary          string
	terminal        Terminal
	sharedCodexHome string
}

func NewCodexProcess(binary string, terminal Terminal) *CodexProcess {
	return &CodexProcess{binary: binary, terminal: terminal}
}

func (process *CodexProcess) SetSharedCodexHome(home string) {
	process.sharedCodexHome = strings.TrimSpace(home)
}

func (process *CodexProcess) Login(
	ctx context.Context,
	codexHome string,
	deviceAuth bool,
) (entity.Identity, error) {
	binary, err := process.resolveBinary()
	if err != nil {
		return entity.Identity{}, err
	}
	if err := prepareCodexHome(codexHome); err != nil {
		return entity.Identity{}, err
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, codexHome)
	if err != nil {
		return entity.Identity{}, err
	}
	defer release()

	arguments := []string{"login"}
	if deviceAuth {
		arguments = append(arguments, "--device-auth")
	}
	command := terminalCommand(ctx, binary, arguments)
	command.Env = environmentWith("CODEX_HOME", codexHome)
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return entity.Identity{}, ctx.Err()
		}
		return entity.Identity{}, fmt.Errorf("codex login failed: %w", err)
	}

	if err := secureCodexHome(codexHome); err != nil {
		return entity.Identity{}, err
	}
	identity, err := readChatGPTIdentity(filepath.Join(codexHome, "auth.json"))
	if err != nil {
		return entity.Identity{}, err
	}
	return identity, nil
}

func (process *CodexProcess) Run(ctx context.Context, codexHome string, arguments []string) error {
	return process.run(ctx, codexHome, arguments)
}

func (process *CodexProcess) run(ctx context.Context, codexHome string, arguments []string) error {
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	if err := secureCodexHomeWithShared(codexHome, process.sharedCodexHome); err != nil {
		return err
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, codexHome)
	if err != nil {
		return err
	}
	defer release()
	command := terminalCommand(ctx, binary, arguments)
	command.Env = codexThreadIndexEnvironment(codexHome, process.sharedCodexHome)
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func terminalCommand(ctx context.Context, binary string, arguments []string) *exec.Cmd {
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Cancel = func() error { return nil }
	command.WaitDelay = 2 * time.Second
	return command
}

func (process *CodexProcess) Version(ctx context.Context) (string, error) {
	binary, err := process.resolveBinary()
	if err != nil {
		return "", err
	}
	home, err := os.MkdirTemp("", "godex-codex-version-")
	if err != nil {
		return "", fmt.Errorf("create Codex version home: %w", err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	command := exec.CommandContext(ctx, binary, "--version")
	command.Env = environmentWith("CODEX_HOME", home)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("run codex --version: %w", err)
	}
	return strings.TrimSpace(output.String()), nil
}

func (process *CodexProcess) resolveBinary() (string, error) {
	candidate := process.binary
	if strings.TrimSpace(candidate) == "" {
		candidate = "codex"
	}
	binary, err := exec.LookPath(candidate)
	if err != nil {
		return "", fmt.Errorf("official Codex CLI %q was not found; install Codex first or set GODEX_CODEX_BIN", candidate)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return "", fmt.Errorf("resolve official Codex CLI path: %w", err)
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", fmt.Errorf("inspect official Codex CLI %q: %w", candidate, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("official Codex CLI %q is not a regular executable", candidate)
	}
	return binary, nil
}

func prepareCodexHome(path string) error {
	if err := validateCodexHomePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create staged Codex home: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect staged Codex home: %w", err)
	}
	if !info.IsDir() {
		return errors.New("staged Codex home is not a directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure staged Codex home: %w", err)
	}
	configPath := filepath.Join(path, "config.toml")
	if err := writePrivateFile(configPath, []byte(codexFileCredentialConfig)); err != nil {
		return fmt.Errorf("write staged Codex config: %w", err)
	}
	return nil
}

func secureCodexHome(path string) error {
	return secureCodexHomeWithShared(path, "")
}

func secureCodexHomeWithShared(path, sharedHome string) error {
	if err := validateCodexHomePath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect Codex home: %w", err)
	}
	if !info.IsDir() {
		return errors.New("codex home is not a directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure Codex home: %w", err)
	}
	for _, name := range []string{"auth.json", "config.toml"} {
		file := filepath.Join(path, name)
		info, err := os.Lstat(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect Codex %s: %w", name, err)
		}
		if name == "config.toml" && info.Mode()&os.ModeSymlink != 0 {
			if err := secureSharedCodexConfig(file, sharedHome); err != nil {
				return fmt.Errorf("secure Codex config.toml: %w", err)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("codex %s is not a regular file", name)
		}
		if err := os.Chmod(file, 0o600); err != nil {
			return fmt.Errorf("secure Codex %s: %w", name, err)
		}
	}
	return nil
}

func secureSharedCodexConfig(linkPath, sharedHome string) error {
	if strings.TrimSpace(sharedHome) == "" {
		return errors.New("shared Codex config link is unavailable without a shared home")
	}
	if err := validateCodexHomePath(sharedHome); err != nil {
		return fmt.Errorf("invalid shared Codex home: %w", err)
	}
	sharedInfo, err := os.Lstat(sharedHome)
	if err != nil {
		return fmt.Errorf("inspect shared Codex home: %w", err)
	}
	if sharedInfo.Mode()&os.ModeSymlink != 0 || !sharedInfo.IsDir() {
		return errors.New("shared Codex home must be a real directory")
	}
	target := filepath.Join(sharedHome, "config.toml")
	linkTarget, err := os.Readlink(linkPath)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(linkPath), linkTarget)
	}
	linkTarget, err = filepath.Abs(linkTarget)
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if filepath.Clean(linkTarget) != filepath.Clean(expected) {
		return errors.New("config.toml points outside the configured shared Codex home")
	}
	targetInfo, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
		return errors.New("shared config.toml must be a real regular file")
	}
	return os.Chmod(target, 0o600)
}

func validateCodexHomePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("codex home path is empty")
	}
	if !filepath.IsAbs(path) {
		return errors.New("codex home path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean == filepath.Dir(clean) {
		return errors.New("codex home path must not be filesystem root")
	}
	return nil
}

func writePrivateFile(path string, content []byte) error {
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("target is not a regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := securePrivateFile(file, path); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func environmentWith(key, value string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		entryKey, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(entryKey, key) {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, key+"="+value)
	return hardenCodexChildEnvironment(environment)
}
