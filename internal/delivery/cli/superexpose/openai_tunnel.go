package superexpose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	openAITunnelMinimumVersion  = "0.0.13"
	openAITunnelLatestReference = "0.0.16"
	openAITunnelProbeTimeout    = 15 * time.Second
	openAITunnelProbeOutputMax  = 1024 * 1024
	openAITunnelReadyTimeout    = 20 * time.Second
	openAITunnelReadyPoll       = 100 * time.Millisecond
	openAITunnelShutdownTimeout = 2 * time.Second
	openAITunnelHealthTimeout   = 750 * time.Millisecond
	openAITunnelHealthURLMax    = 4096
	openAITunnelAPIKeyMax       = 4096
)

var (
	openAITunnelIDPattern = regexp.MustCompile("^tunnel_[a-z0-9]{32}$")
	nextTunnelConfigID    atomic.Uint64
)

type openAITunnelProcess struct {
	command   *exec.Cmd
	id        string
	version   string
	directory string
	healthURL string
	exit      <-chan error
}

func validateOpenAITunnelID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !openAITunnelIDPattern.MatchString(value) {
		return "", errors.New("OpenAI tunnel id must match tunnel_<32 lowercase letters or digits>")
	}
	return value, nil
}

func ensureOpenAITunnelAvailable() (string, string, error) {
	binary, err := openAITunnelBinary()
	if err != nil {
		return "", "", err
	}
	versionOutput, err := probeTunnelCommand(binary, "--version")
	if err != nil {
		return "", "", openAITunnelInstallError()
	}
	version, ok := supportedTunnelClientVersion(versionOutput)
	if !ok {
		return "", "", fmt.Errorf(
			"tunnel-client did not report a compatible official stable build; Godex requires %s or newer (release-qualified reference: %s)",
			openAITunnelMinimumVersion, openAITunnelLatestReference,
		)
	}
	if _, err := probeTunnelCommand(binary, "run", "--help"); err != nil {
		return "", "", fmt.Errorf("tunnel-client %s does not expose the required run capability", version)
	}
	return binary, version, nil
}

func openAITunnelBinary() (string, error) {
	for _, key := range []string{"GODEX_TUNNEL_CLIENT_BIN", "PRODEX_TUNNEL_CLIENT_BIN"} {
		if configured := strings.TrimSpace(os.Getenv(key)); configured != "" {
			path, err := exec.LookPath(configured)
			if err == nil {
				return path, nil
			}
			if filepath.IsAbs(configured) {
				if info, statErr := os.Stat(configured); statErr == nil && info.Mode().IsRegular() {
					return configured, nil
				}
			}
			return "", openAITunnelInstallError()
		}
	}
	path, err := exec.LookPath("tunnel-client")
	if err != nil {
		return "", openAITunnelInstallError()
	}
	return path, nil
}

func openAITunnelInstallError() error {
	return fmt.Errorf(
		"tunnel-client is required for OpenAI Secure MCP Tunnel mode; install or update official openai/tunnel-client (minimum supported: %s; release-qualified reference: %s)",
		openAITunnelMinimumVersion, openAITunnelLatestReference,
	)
}

func probeTunnelCommand(binary string, args ...string) (string, error) {
	command := exec.Command(binary, args...)
	command.Env = tunnelProbeEnvironment()
	command.Stdin = nil
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", errors.New("tunnel-client probe failed")
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return "", errors.New("tunnel-client probe failed")
	}
	configureExecProcess(command)
	if err := command.Start(); err != nil {
		return "", errors.New("tunnel-client probe failed")
	}

	type readResult struct {
		bytes []byte
		err   error
	}
	readBounded := func(reader io.Reader) <-chan readResult {
		result := make(chan readResult, 1)
		go func() {
			bytes, err := io.ReadAll(io.LimitReader(reader, int64(openAITunnelProbeOutputMax)+1))
			result <- readResult{bytes: bytes, err: err}
			close(result)
		}()
		return result
	}
	stdoutResult := readBounded(stdout)
	stderrResult := readBounded(stderr)
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
		close(waitResult)
	}()

	timer := time.NewTimer(openAITunnelProbeTimeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-waitResult:
	case <-timer.C:
		stopExecProcessTree(command)
		<-waitResult
		return "", errors.New("tunnel-client probe failed")
	}

	stdoutRead := <-stdoutResult
	stderrRead := <-stderrResult
	if waitErr != nil || stdoutRead.err != nil || stderrRead.err != nil ||
		len(stdoutRead.bytes) > openAITunnelProbeOutputMax || len(stderrRead.bytes) > openAITunnelProbeOutputMax {
		return "", errors.New("tunnel-client probe failed")
	}
	return string(stdoutRead.bytes) + "\n" + string(stderrRead.bytes), nil
}

func tunnelProbeEnvironment() []string {
	environment := inheritedEnvironment()
	removeInheritedTunnelConfiguration(environment)
	return flattenEnvironment(environment)
}

func supportedTunnelClientVersion(text string) (string, bool) {
	minimum, _ := parseExposeTunnelSemver(openAITunnelMinimumVersion)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		versionAndSHA, suffix, ok := strings.Cut(line, " (git sha: ")
		if !ok || !strings.HasSuffix(suffix, ")") {
			continue
		}
		suffix = strings.TrimSuffix(suffix, ")")
		versionText, sha, ok := strings.Cut(versionAndSHA, "+")
		if !ok || sha != suffix || !validTunnelGitSHA(sha) {
			continue
		}
		version, ok := parseExposeTunnelSemver(versionText)
		if !ok || version.pre != "" || compareExposeTunnelSemver(version, minimum) < 0 {
			continue
		}
		return versionText, true
	}
	return "", false
}

type exposeTunnelSemver struct {
	major, minor, patch uint64
	pre                 string
}

func parseExposeTunnelSemver(value string) (exposeTunnelSemver, bool) {
	core, pre, _ := strings.Cut(strings.TrimSpace(value), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return exposeTunnelSemver{}, false
	}
	var values [3]uint64
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return exposeTunnelSemver{}, false
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return exposeTunnelSemver{}, false
		}
		values[index] = parsed
	}
	return exposeTunnelSemver{major: values[0], minor: values[1], patch: values[2], pre: pre}, true
}

func compareExposeTunnelSemver(left, right exposeTunnelSemver) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func validTunnelGitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, current := range value {
		if !(current >= '0' && current <= '9') &&
			!(current >= 'a' && current <= 'f') &&
			!(current >= 'A' && current <= 'F') {
			return false
		}
	}
	return true
}

func startOpenAITunnel(localMCPURL, tunnelID string) (*openAITunnelProcess, error) {
	tunnelID, err := validateOpenAITunnelID(tunnelID)
	if err != nil {
		return nil, err
	}
	binary, version, err := ensureOpenAITunnelAvailable()
	if err != nil {
		return nil, err
	}
	return spawnOpenAITunnel(localMCPURL, tunnelID, binary, version)
}

func createOpenAITunnelFiles(localMCPURL string) (string, error) {
	unique := nextTunnelConfigID.Add(1)
	var directory string
	for attempt := 0; attempt < 16; attempt++ {
		candidate := filepath.Join(os.TempDir(), fmt.Sprintf("godex-openai-tunnel-%d-%d-%d", os.Getpid(), unique, attempt))
		err := os.Mkdir(candidate, 0o700)
		if err == nil {
			directory = candidate
			break
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return "", fmt.Errorf("failed to create private OpenAI tunnel directory: %w", err)
	}
	if directory == "" {
		return "", errors.New("failed to create private OpenAI tunnel directory")
	}
	fail := func(err error) (string, error) {
		_ = os.RemoveAll(directory)
		return "", err
	}
	mcpURL := filepath.Join(directory, "mcp-url")
	healthURL := filepath.Join(directory, "health-url")
	config := filepath.Join(directory, "config.yaml")
	for _, path := range []string{mcpURL, healthURL, config} {
		if !utf8.ValidString(path) || strings.IndexFunc(path, unicode.IsControl) >= 0 {
			return fail(errors.New("private OpenAI tunnel path is not safe for configuration"))
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return fail(err)
		}
	}
	if err := os.WriteFile(mcpURL, []byte(localMCPURL), 0o600); err != nil {
		return fail(err)
	}
	configText := "config_version: 1\nmcp:\n  server_urls:\n    - channel: main\n      url: '" +
		strings.ReplaceAll("file:"+mcpURL, "'", "''") + "'\n"
	if err := os.WriteFile(config, []byte(configText), 0o600); err != nil {
		return fail(err)
	}
	return directory, nil
}

func validateLocalTunnelMCPURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) ||
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return "", errors.New("OpenAI tunnel MCP endpoint must be a loopback HTTP URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Port() == "" || parsed.Port() == "0" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("OpenAI tunnel MCP endpoint must be a loopback HTTP URL")
	}
	host := net.ParseIP(parsed.Hostname())
	if host == nil || !host.IsLoopback() {
		return "", errors.New("OpenAI tunnel MCP endpoint must be a loopback HTTP URL")
	}
	return value, nil
}

func removeInheritedTunnelConfiguration(environment map[string]string) {
	for _, variable := range []string{
		"OPENAI_API_KEY", "OPENAI_API_KEYS", "OPENAI_ADMIN_KEY", "CONTROL_PLANE_API_KEY",
		"TUNNEL_CLIENT_CONFIG", "TUNNEL_CLIENT_PROFILE", "TUNNEL_CLIENT_PROFILE_FILE", "TUNNEL_CLIENT_PROFILE_DIR",
		"CONTROL_PLANE_BASE_URL", "CONTROL_PLANE_URL_PATH", "CONTROL_PLANE_TUNNEL_ID", "CONTROL_PLANE_POLL_CHANNELS",
		"MCP_SERVER_URL", "MCP_COMMAND", "HEALTH_LISTEN_ADDR", "HEALTH_UNIX_SOCKET", "HEALTH_URL_FILE",
		"LOG_FILE", "LOG_HTTP_RAW_UNSAFE", "CLOUDFLARED_TUNNEL_TOKEN", "CLOUDFLARED_MANAGED",
		"CLOUDFLARED_PATH", "CLOUDFLARED_READY_TIMEOUT", "TUNNEL_CONFIG", "TUNNEL_CERT",
		"TUNNEL_ORIGIN_CERT", "TUNNEL_CRED_FILE", "TUNNEL_HOSTNAME", "TUNNEL_NAME", "TUNNEL_TOKEN",
		"TUNNEL_TRANSPORT_PROTOCOL",
	} {
		delete(environment, variable)
	}
}

func (tunnel *openAITunnelProcess) waitReady(ctx context.Context) error {
	deadline := time.NewTimer(openAITunnelReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(openAITunnelReadyPoll)
	defer ticker.Stop()
	var healthBase string
	for {
		select {
		case err := <-tunnel.exit:
			if err == nil {
				return errors.New("tunnel-client exited before local readiness completed")
			}
			return fmt.Errorf("tunnel-client exited before local readiness completed: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("tunnel-client did not become locally ready within 20 seconds")
		case <-ticker.C:
			if healthBase == "" {
				value, ok, err := readTunnelHealthBase(tunnel.healthURL)
				if err != nil {
					return err
				}
				if ok {
					healthBase = value
				}
			}
			if healthBase != "" && tunnelHealthReady(healthBase) {
				select {
				case err := <-tunnel.exit:
					if err == nil {
						return errors.New("tunnel-client exited before local readiness completed")
					}
					return fmt.Errorf("tunnel-client exited before local readiness completed: %w", err)
				default:
				}
				return nil
			}
		}
	}
}

func readTunnelHealthBase(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Size() == 0 {
		return "", false, nil
	}
	if info.Size() > openAITunnelHealthURLMax {
		return "", false, errors.New("tunnel-client health URL file is too large")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if !utf8.Valid(content) {
		return "", false, errors.New("tunnel-client health URL file is not valid UTF-8")
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", false, nil
	}
	parsed, err := validateTunnelHealthBase(value)
	return parsed, err == nil, err
}

func validateTunnelHealthBase(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" || !utf8.ValidString(value) ||
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return "", errors.New("tunnel-client health URL must be a loopback HTTP base URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Port() == "" || parsed.Port() == "0" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("tunnel-client health URL must be a loopback HTTP base URL")
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("tunnel-client health URL must be a loopback HTTP base URL")
	}
	return value, nil
}

func tunnelHealthReady(baseURL string) bool {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{
		Transport:     transport,
		Timeout:       openAITunnelHealthTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		response, err := client.Get(baseURL + path)
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return false
		}
	}
	return true
}

func (tunnel *openAITunnelProcess) shutdown() {
	if tunnel == nil {
		return
	}
	stopExecProcessTree(tunnel.command)
	timer := time.NewTimer(openAITunnelShutdownTimeout)
	defer timer.Stop()
	select {
	case <-tunnel.exit:
	case <-timer.C:
		if tunnel.command != nil && tunnel.command.Process != nil {
			_ = tunnel.command.Process.Kill()
		}
	}
	if tunnel.directory != "" {
		_ = os.RemoveAll(tunnel.directory)
	}
}

func tunnelExitCodeLabel(err error) string {
	if err == nil {
		return "0"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		if code := exitErr.ProcessState.ExitCode(); code >= 0 {
			return fmt.Sprint(code)
		}
		return "signal"
	}
	return "unknown"
}

func tunnelHasControl(value string) bool {
	for _, current := range value {
		if unicode.IsControl(current) {
			return true
		}
	}
	return false
}
