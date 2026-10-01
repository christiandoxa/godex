package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	metadataTimeout        = 5 * time.Second
	metadataOutputMaxBytes = 1 << 20
)

type metadataResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type metadataRunner func(context.Context, string, []string, map[string]string) (metadataResult, error)

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

func runMetadataCommand(ctx context.Context, binary string, arguments []string, overrides map[string]string) (metadataResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, arguments...)
	command.Env = mergedEnvironment(overrides)
	stdout := &boundedBuffer{limit: metadataOutputMaxBytes}
	stderr := &boundedBuffer{limit: metadataOutputMaxBytes}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			return metadataResult{}, ctx.Err()
		}
		return metadataResult{}, errors.New("Kiro metadata command timed out")
	}
	if stdout.truncated || stderr.truncated {
		return metadataResult{}, errors.New("Kiro metadata command exceeded the output limit")
	}
	result := metadataResult{stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return metadataResult{}, fmt.Errorf("run Kiro metadata command: %w", err)
}

func (source *Source) readWhoami(ctx context.Context, databasePath string) (map[string]any, error) {
	result, err := source.run(ctx, source.binary(), []string{"whoami", "--format", "json"}, source.databaseEnvironment(databasePath, ""))
	if err != nil {
		return nil, err
	}
	if result.exitCode != 0 {
		return nil, errors.New("Kiro CLI whoami failed")
	}
	var value map[string]any
	if err := decodeJSON(result.stdout, &value); err != nil {
		return nil, errors.New("failed to parse Kiro whoami JSON")
	}
	return value, nil
}

func (source *Source) readModelCatalog(ctx context.Context, databasePath, region string) (string, error) {
	result, err := source.run(ctx, source.binary(), []string{"chat", "--list-models", "--format", "json"}, source.databaseEnvironment(databasePath, region))
	if err != nil {
		return "", err
	}
	if result.exitCode != 0 {
		return "", errors.New("Kiro model catalog command failed")
	}
	return normalizeModelCatalogText(string(result.stdout))
}

func (source *Source) binary() string {
	if override := strings.TrimSpace(source.getenv("PRODEX_KIRO_BIN")); override != "" {
		return override
	}
	for _, candidate := range []string{"kiro-cli-chat", "kiro-cli"} {
		if path, err := source.lookupPath(candidate); err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}
	return "kiro-cli-chat"
}

func (source *Source) databaseEnvironment(databasePath, region string) map[string]string {
	dataDir := filepath.Dir(databasePath)
	environment := map[string]string{
		"KIRO_DATA_DIR": dataDir, "Q_CLI_DATA_DIR": dataDir, "KIRO_TEST_DB_PATH": databasePath,
	}
	if strings.TrimSpace(region) != "" {
		environment["AWS_REGION"] = strings.TrimSpace(region)
	}
	return environment
}

func mergedEnvironment(overrides map[string]string) []string {
	keys := make(map[string]bool, len(overrides))
	for key := range overrides {
		keys[key] = true
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found && !keys[key] {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func decodeJSON(content []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	return decoder.Decode(destination)
}
