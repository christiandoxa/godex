package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

const (
	pingTimeout        = 45 * time.Second
	pingOutputMaxBytes = 1 << 20
)

var providerSecretEnvKeys = map[string]bool{
	"OPENAI_API_KEYS": true, "OPENAI_API_KEY": true,
	"ANTHROPIC_API_KEYS": true, "ANTHROPIC_API_KEY": true,
	"DEEPSEEK_API_KEYS": true, "DEEPSEEK_API_KEY": true,
	"GEMINI_API_KEYS": true, "GEMINI_API_KEY": true,
	"GOOGLE_API_KEYS": true, "GOOGLE_API_KEY": true,
	"GITHUB_COPILOT_API_KEYS": true, "GITHUB_COPILOT_API_KEY": true,
}

var upstreamProxyEnvKeys = map[string]bool{
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true,
	"http_proxy": true, "https_proxy": true, "all_proxy": true,
	"PROXY": true, "proxy": true,
}

var dangerousChildEnvKeys = map[string]bool{
	"LD_PRELOAD": true, "LD_AUDIT": true, "LD_LIBRARY_PATH": true, "LD_ORIGIN_PATH": true,
	"DYLD_INSERT_LIBRARIES": true, "DYLD_LIBRARY_PATH": true, "DYLD_FRAMEWORK_PATH": true,
}

func (process *CodexProcess) PingOpenAI(ctx context.Context, target pingmodel.Target, options pingmodel.Options) (pingmodel.ProcessResult, error) {
	binary, err := process.resolveBinary()
	if err != nil {
		return pingmodel.ProcessResult{}, err
	}
	if err := secureCodexHomeWithShared(target.CodexHome, process.sharedCodexHome); err != nil {
		return pingmodel.ProcessResult{}, err
	}
	arguments, err := pingModelContextArguments(target.CodexHome, pingArguments(options))
	if err != nil {
		return pingmodel.ProcessResult{}, err
	}
	cwd, err := os.MkdirTemp("", "godex-ping-")
	if err != nil {
		return pingmodel.ProcessResult{}, fmt.Errorf("create ping diagnostic directory: %w", err)
	}
	if err := os.Chmod(cwd, 0o700); err != nil {
		_ = cleanupPingDirectory(cwd)
		return pingmodel.ProcessResult{}, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	started := time.Now()
	stdout := newPingCapture(started, true)
	stderr := newPingCapture(started, false)
	command := exec.CommandContext(pingCtx, binary, arguments...)
	command.Dir = cwd
	command.Env = pingEnvironment(target.CodexHome, options.NoProxy)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		cleanupErr := cleanupPingDirectory(cwd)
		return pingmodel.ProcessResult{}, errors.Join(fmt.Errorf("failed to start Codex ping: %w", err), cleanupErr)
	}
	waitErr := command.Wait()
	elapsed := time.Since(started).Milliseconds()
	result := pingmodel.ProcessResult{
		Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: 0,
		FirstResponseMS: stdout.FirstResponseMS(), LatencyMS: elapsed,
		CleanupFailed: cleanupPingDirectory(cwd) != nil,
	}
	if pingCtx.Err() != nil {
		result.Cancelled = ctx.Err() != nil
		result.TimedOut = ctx.Err() == nil && errors.Is(pingCtx.Err(), context.DeadlineExceeded)
		return result, nil
	}
	if waitErr == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("wait for Codex ping: %w", waitErr)
}

func cleanupPingDirectory(path string) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.RemoveAll(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if runtime.GOOS != "windows" || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func pingArguments(options pingmodel.Options) []string {
	arguments := []string{
		"exec", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config",
		"--ignore-rules", "--skip-git-repo-check", "-c", `model_provider="openai"`,
	}
	if options.BaseURL != "" {
		arguments = append(arguments, "-c", "chatgpt_base_url="+strconv.Quote(options.BaseURL))
	}
	if options.Model != "" {
		arguments = append(arguments, "--model", options.Model)
	}
	return append(arguments, "--json", "--color", "never", "hello")
}

func pingEnvironment(codexHome string, noProxy bool) []string {
	removed := make(map[string]bool, len(providerSecretEnvKeys)+len(dangerousChildEnvKeys)+len(upstreamProxyEnvKeys))
	for key := range providerSecretEnvKeys {
		removed[key] = true
	}
	for key := range dangerousChildEnvKeys {
		removed[key] = true
	}
	if noProxy {
		for key := range upstreamProxyEnvKeys {
			removed[key] = true
		}
	}
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if !found || strings.EqualFold(key, "CODEX_HOME") || removed[key] {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "CODEX_HOME="+filepath.Clean(codexHome))
}

type pingCapture struct {
	mu        sync.Mutex
	started   time.Time
	watch     bool
	buffer    bytes.Buffer
	line      bytes.Buffer
	firstMS   *int64
	truncated bool
}

func newPingCapture(started time.Time, watch bool) *pingCapture {
	return &pingCapture{started: started, watch: watch}
}

func (capture *pingCapture) Write(content []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.captureBytes(content)
	if capture.watch {
		capture.captureLines(content)
	}
	return len(content), nil
}

func (capture *pingCapture) captureBytes(content []byte) {
	remaining := pingOutputMaxBytes - capture.buffer.Len()
	if remaining <= 0 {
		capture.truncated = true
		return
	}
	if len(content) > remaining {
		content = content[:remaining]
		capture.truncated = true
	}
	_, _ = capture.buffer.Write(content)
}

func (capture *pingCapture) captureLines(content []byte) {
	for _, current := range content {
		if current == '\n' {
			capture.observeLine(capture.line.Bytes())
			capture.line.Reset()
			continue
		}
		if capture.line.Len() < 256<<10 {
			_ = capture.line.WriteByte(current)
		}
	}
}

func (capture *pingCapture) observeLine(line []byte) {
	if capture.firstMS != nil || !pingLineIsAgentMessage(line) {
		return
	}
	elapsed := time.Since(capture.started).Milliseconds()
	capture.firstMS = &elapsed
}

func pingLineIsAgentMessage(line []byte) bool {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &event) != nil {
		return false
	}
	return event.Type == "item.completed" && event.Item.Type == "agent_message" && strings.TrimSpace(event.Item.Text) != ""
}

func (capture *pingCapture) Bytes() []byte {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]byte(nil), capture.buffer.Bytes()...)
}

func (capture *pingCapture) FirstResponseMS() *int64 {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.firstMS == nil {
		return nil
	}
	value := *capture.firstMS
	return &value
}
