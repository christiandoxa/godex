package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const startupUsagePath = "/backend-api/wham/usage"

type startupProbeCase struct {
	name        string
	status      int
	wantState   bool
	response    string
	reloadOn401 bool
	config      string
	residency   string
}

type startupProbeResult struct {
	exitCode int
	requests []startupProbeRequest
	snapshot bool
}

type startupProbeRequest struct {
	method    string
	path      string
	authValid bool
	residency string
}

type usageServer struct {
	server *httptest.Server
	status int
	body   string

	mu         sync.Mutex
	requestLog []startupProbeRequest
}

type usageSnapshotValue struct {
	PlanType                 string `json:"plan_type"`
	FiveHourStatus           string `json:"five_hour_status"`
	FiveHourRemainingPercent int64  `json:"five_hour_remaining_percent"`
	WeeklyStatus             string `json:"weekly_status"`
	WeeklyRemainingPercent   int64  `json:"weekly_remaining_percent"`
}

var startupEndpointPattern = regexp.MustCompile(`http://127\.0\.0\.1:\d+/backend-api/[^[:space:]]+`)

func checkStartupWarmup(root string, opts cliOptions) ([]stepResult, error) {
	cases := []startupProbeCase{
		{name: "startup_usage_success", status: http.StatusOK, wantState: true,
			response: `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":20,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":40,"reset_at":4102444800,"limit_window_seconds":604800}}}`},
		{name: "startup_usage_failure", status: http.StatusServiceUnavailable, wantState: false,
			response: `{"error":{"code":"synthetic_unavailable"}}`},
		{name: "startup_usage_401_credential_reload", status: http.StatusOK, wantState: true, reloadOn401: true,
			response: `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":20,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":40,"reset_at":4102444800,"limit_window_seconds":604800}}}`},
	}
	for _, item := range []struct{ name, value, want string }{
		{"us", "us", "us"}, {"uppercase", " US ", "us"},
		{"other", "eu", ""}, {"confusable", "uſ", ""},
	} {
		probe := cases[0]
		probe.name = "startup_residency_" + item.name
		probe.config = "enforce_residency='" + item.value + "'\n"
		probe.residency = item.want
		cases = append(cases, probe)
	}
	results := make([]stepResult, 0, len(cases))
	for _, probeCase := range cases {
		prodex, err := runStartupProbeCase(root, opts.prodex, "prodex", probeCase)
		if err != nil {
			return nil, fmt.Errorf("Prodex %s: %w", probeCase.name, err)
		}
		godex, err := runStartupProbeCase(root, opts.godex, "godex", probeCase)
		if err != nil {
			return nil, fmt.Errorf("Godex %s: %w", probeCase.name, err)
		}
		if !sameStartupProbeResult(prodex, godex, probeCase) {
			return nil, fmt.Errorf("%s diverged: Prodex=%+v Godex=%+v", probeCase.name, prodex, godex)
		}
		results = append(results, stepResult{
			Name: probeCase.name, ProdexExit: prodex.exitCode, GodexExit: godex.exitCode,
			Requests: len(prodex.requests), Snapshot: prodex.snapshot,
		})
	}
	return results, nil
}

func sameStartupProbeResult(left, right startupProbeResult, probeCase startupProbeCase) bool {
	wantRequests := 1
	if probeCase.reloadOn401 {
		wantRequests = 2
	}
	if left.exitCode != right.exitCode || len(left.requests) != len(right.requests) ||
		len(left.requests) != wantRequests || left.snapshot != right.snapshot || left.snapshot != probeCase.wantState {
		return false
	}
	canonical := startupProbeRequest{method: http.MethodGet, path: startupUsagePath, authValid: true, residency: probeCase.residency}
	for index := 0; index < wantRequests; index++ {
		if left.requests[index] != canonical || right.requests[index] != canonical {
			return false
		}
	}
	return true
}

func runStartupProbeCase(root, binary, product string, probeCase startupProbeCase) (startupProbeResult, error) {
	directory := filepath.Join(root, "startup", product, probeCase.name)
	userHome := filepath.Join(directory, "user")
	configHome := filepath.Join(directory, product)
	currentHome := filepath.Join(userHome, ".codex")
	for _, path := range []string{userHome, currentHome} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return startupProbeResult{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(currentHome, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-access","account_id":"synthetic-account"}}`), 0o600); err != nil {
		return startupProbeResult{}, err
	}
	if probeCase.config != "" {
		if err := os.WriteFile(filepath.Join(currentHome, "config.toml"), []byte(probeCase.config), 0o600); err != nil {
			return startupProbeResult{}, err
		}
	}
	fixture := profileFixture{
		name: product, binary: binary, home: userHome, configHome: configHome,
		env: startupEnvironment(userHome, currentHome, configHome),
	}
	if code, output, err := invokeProfileCommand(fixture, []string{"profile", "import-current", "main", "--insecure"}); err != nil {
		return startupProbeResult{}, err
	} else if code != 0 {
		return startupProbeResult{}, fmt.Errorf("import-current exit=%d output=%q", code, output)
	}
	usage := newUsageServer(probeCase.status, probeCase.response, "synthetic-account")
	if probeCase.reloadOn401 {
		importedAuth, locateErr := startupManagedAuthFile043714(configHome)
		if locateErr != nil {
			usage.Close()
			return startupProbeResult{}, locateErr
		}
		usage.Close()
		usage = newRotatingUsageServer043714(importedAuth, probeCase.response)
	}
	defer usage.Close()
	result, err := runStartupGateway(fixture, usage.server.URL+"/backend-api", probeCase.wantState)
	if err != nil {
		return startupProbeResult{}, fmt.Errorf("%w; observed quota requests=%+v", err, usage.requests())
	}
	result.requests = usage.requests()
	result.snapshot, err = validateUsageSnapshot(configHome, probeCase.wantState)
	if err != nil {
		return startupProbeResult{}, err
	}
	return result, nil
}

func startupEnvironment(userHome, currentHome, configHome string) []string {
	return []string{
		"HOME=" + userHome, "CODEX_HOME=" + currentHome,
		"PRODEX_HOME=" + configHome, "GODEX_HOME=" + configHome,
		"PRODEX_CODEX_BIN=/bin/true", "GODEX_CODEX_BIN=/bin/true",
		"XDG_CONFIG_HOME=" + filepath.Join(userHome, ".config"),
		"PATH=" + os.Getenv("PATH"), "NO_COLOR=1", "CI=1",
		"PRODEX_NO_UPDATE_CHECK=1", "GODEX_NO_UPDATE_CHECK=1",
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=127.0.0.1,localhost",
	}
}

func newUsageServer(status int, body, accountID string) *usageServer {
	usage := &usageServer{status: status, body: body}
	usage.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		usage.mu.Lock()
		usage.requestLog = append(usage.requestLog, startupProbeRequest{
			method: request.Method, path: request.URL.Path,
			residency: request.Header.Get("x-openai-internal-codex-residency"),
			authValid: request.Header.Get("Authorization") == "Bearer synthetic-access" &&
				request.Header.Get("ChatGPT-Account-Id") == accountID &&
				request.Header.Get("originator") == "codex_cli_rs" &&
				request.Header.Get("x-openai-codex-luna-reserve") == "1",
		})
		usage.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(usage.status)
		_, _ = writer.Write([]byte(usage.body))
	}))
	return usage
}

// The official tagged quota client reloads an auth.json changed after the
// first HTTP 401, not blindly resending the original bearer. Keep the
// mutation inside the loopback fixture and never use real provider tokens.
func newRotatingUsageServer043714(authPath, successBody string) *usageServer {
	usage := &usageServer{status: http.StatusOK, body: successBody}
	initialBearer := ""
	usage.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usage.mu.Lock()
		index := len(usage.requestLog)
		bearer := r.Header.Get("Authorization")
		validEnvelope := r.Method == http.MethodGet && r.URL.Path == startupUsagePath &&
			r.Header.Get("ChatGPT-Account-Id") == "synthetic-account" &&
			r.Header.Get("originator") == "codex_cli_rs" &&
			r.Header.Get("x-openai-codex-luna-reserve") == "1"
		valid := validEnvelope && ((index == 0 && strings.HasPrefix(bearer, "Bearer ") && len(bearer) > len("Bearer ")) ||
			(index > 0 && bearer == "Bearer synthetic-rotated" && bearer != initialBearer))
		if index == 0 && valid {
			initialBearer = bearer
		}
		usage.requestLog = append(usage.requestLog, startupProbeRequest{
			method: r.Method, path: r.URL.Path, authValid: valid,
		})
		usage.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if index == 0 {
			if valid {
				next := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-rotated","account_id":"synthetic-account"}}`)
				temp := authPath + ".next"
				if err := os.WriteFile(temp, next, 0600); err != nil {
					w.WriteHeader(500)
					return
				}
				if err := os.Rename(temp, authPath); err != nil {
					w.WriteHeader(500)
					return
				}
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successBody))
	}))
	return usage
}

func (usage *usageServer) requests() []startupProbeRequest {
	usage.mu.Lock()
	defer usage.mu.Unlock()
	return append([]startupProbeRequest(nil), usage.requestLog...)
}

func (usage *usageServer) Close() { usage.server.Close() }

func runStartupGateway(fixture profileFixture, baseURL string, wantSnapshot bool) (startupProbeResult, error) {
	command := exec.Command(fixture.binary, "gateway", "--listen", "127.0.0.1:0", "--base-url", baseURL)
	command.Dir = fixture.home
	command.Env = fixture.env
	stdout, err := command.StdoutPipe()
	if err != nil {
		return startupProbeResult{}, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return startupProbeResult{}, err
	}
	endpoint := make(chan string, 1)
	go scanStartupEndpoint(stdout, endpoint)
	select {
	case <-endpoint:
		if wantSnapshot {
			if err := waitForUsageSnapshotFile(fixture.configHome); err != nil {
				_ = command.Process.Kill()
				_ = command.Wait()
				return startupProbeResult{}, err
			}
		}
	case <-time.After(8 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		return startupProbeResult{}, fmt.Errorf("gateway did not start: %s", stderr.String())
	}
	waitErr := stopStartupGateway(command)
	if waitErr != nil && command.ProcessState.ExitCode() >= 0 {
		return startupProbeResult{}, fmt.Errorf("gateway exit=%d: %w; stderr=%s", command.ProcessState.ExitCode(), waitErr, stderr.String())
	}
	return startupProbeResult{exitCode: 0}, nil
}

func waitForUsageSnapshotFile(configHome string) error {
	path := filepath.Join(configHome, "runtime-usage-snapshots.json")
	deadline := time.Now().Add(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("usage snapshot was not written: %s", path)
		}
		<-ticker.C
	}
}

func scanStartupEndpoint(reader io.Reader, endpoint chan<- string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if match := startupEndpointPattern.FindString(scanner.Text()); match != "" {
			endpoint <- match
			return
		}
	}
}

func stopStartupGateway(command *exec.Cmd) error {
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	if err := command.Process.Signal(os.Interrupt); err != nil {
		_ = command.Process.Kill()
	}
	select {
	case err := <-wait:
		return err
	case <-time.After(8 * time.Second):
		_ = command.Process.Kill()
		return <-wait
	}
}

func validateUsageSnapshot(configHome string, wantSnapshot bool) (bool, error) {
	path := filepath.Join(configHome, "runtime-usage-snapshots.json")
	deadline := time.Now().Add(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			if !wantSnapshot {
				return false, errors.New("unexpected usage snapshot after failed probe")
			}
			if err := validateUsageSnapshotJSON(raw); err != nil {
				return false, err
			}
			return true, nil
		}
		if !wantSnapshot && errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("usage snapshot availability: %w", err)
		}
		<-ticker.C
	}
}

func validateUsageSnapshotJSON(raw []byte) error {
	var envelope struct {
		Generation int                           `json:"generation"`
		Value      map[string]usageSnapshotValue `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("invalid usage snapshot: %w", err)
	}
	if envelope.Generation < 1 || len(envelope.Value) != 1 {
		return fmt.Errorf("unexpected usage snapshot generation/value: %d/%d", envelope.Generation, len(envelope.Value))
	}
	for _, value := range envelope.Value {
		if value.PlanType != "plus" || value.FiveHourStatus != "Ready" || value.FiveHourRemainingPercent != 80 ||
			value.WeeklyStatus != "Ready" || value.WeeklyRemainingPercent != 60 {
			return fmt.Errorf("unexpected usage snapshot value: %+v", value)
		}
	}
	return nil
}
