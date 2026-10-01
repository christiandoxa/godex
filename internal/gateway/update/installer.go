package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"

	godexassets "github.com/christiandoxa/godex"
	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

const updateOutputMaxBytes = 64 << 10
const updateRenderedMaxRunes = 16 << 10

type Installer struct {
	executable func() (string, error)
}

func NewInstaller() *Installer { return &Installer{executable: os.Executable} }

func (installer *Installer) CurrentExecutable() (string, error) {
	path, err := installer.executable()
	if err != nil {
		return "", errors.New("failed to locate current Godex binary")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("failed to resolve current Godex binary")
	}
	return filepath.Clean(absolute), nil
}

func (installer *Installer) ProbeVersion(ctx context.Context, path string) (string, error) {
	command := exec.CommandContext(ctx, path, "--version")
	var output cappedBuffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return "", errors.New("installed Godex version probe failed")
	}
	fields := strings.Fields(output.String())
	if len(fields) < 2 || fields[0] != "godex" {
		return "", errors.New("installed Godex version probe returned unexpected output")
	}
	return strings.TrimPrefix(fields[1], "v"), nil
}

func (installer *Installer) Install(ctx context.Context, runningExe, targetVersion string) (updatemodel.InstallResult, error) {
	command, script, err := updateCommand(ctx)
	if err != nil {
		return updatemodel.InstallResult{}, err
	}
	command.Stdin = bytes.NewReader(script)
	command.Env = updateEnvironment(runningExe, targetVersion)
	var stdout, stderr cappedBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	runErr := command.Run()
	result := updatemodel.InstallResult{Stdout: boundedUpdateOutput(stdout.String()), Stderr: boundedUpdateOutput(stderr.String())}
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			return result, fmt.Errorf("Godex installer exited with code %d", exitError.ExitCode())
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("failed to run Godex installer")
	}
	return result, nil
}

func updateCommand(ctx context.Context) (*exec.Cmd, []byte, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return exec.CommandContext(ctx, "sh", "-s", "--"), godexassets.InstallSH, nil
	case "windows":
		return exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "-"), godexassets.InstallPowerShell, nil
	default:
		return nil, nil, fmt.Errorf("godex update supports macOS, Linux, and Windows; current OS is %s", runtime.GOOS)
	}
}

func updateEnvironment(runningExe, targetVersion string) []string {
	removed := map[string]bool{
		"GODEX_VERSION": true, "GODEX_INSTALL_DIR": true, "GODEX_RUNNING_EXE": true,
		"GODEX_MIGRATE": true, "GODEX_NON_INTERACTIVE": true,
	}
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && removed[strings.ToUpper(key)] {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"GODEX_VERSION="+targetVersion,
		"GODEX_INSTALL_DIR="+filepath.Dir(runningExe),
		"GODEX_RUNNING_EXE="+runningExe,
		"GODEX_MIGRATE=1",
		"GODEX_NON_INTERACTIVE=1",
	)
}

type cappedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	truncated bool
}

func (buffer *cappedBuffer) Write(content []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := updateOutputMaxBytes - buffer.buffer.Len()
	if remaining > 0 {
		stored := content
		if len(stored) > remaining {
			stored = stored[:remaining]
			buffer.truncated = true
		}
		_, _ = buffer.buffer.Write(stored)
	} else {
		buffer.truncated = true
	}
	return len(content), nil
}

func (buffer *cappedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

var updateSecretPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(["']?(?:access_token|refresh_token|id_token|api_key|token)["']?\s*[:=]\s*["']?)[^"',\s}]+`), `${1}<redacted>`},
	{regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`), `${1}<redacted>@`},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`), `<redacted>`},
}

func boundedUpdateOutput(value string) string {
	for _, secret := range updateSecretPatterns {
		value = secret.pattern.ReplaceAllString(value, secret.replace)
	}
	filtered := strings.Map(func(current rune) rune {
		if unicode.IsControl(current) && current != '\n' && current != '\t' {
			return -1
		}
		return current
	}, value)
	runes := []rune(filtered)
	if len(runes) <= updateRenderedMaxRunes {
		return strings.TrimSpace(filtered)
	}
	return strings.TrimSpace(string(runes[:updateRenderedMaxRunes])) + "\n… output truncated …"
}
