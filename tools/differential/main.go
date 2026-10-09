package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	prodexCommit = "4c61dc0a84a6cf4852feb08e3a8406c1846bb5ad"
	apiKey       = "synthetic-provider-key"
	bodyLimit    = 1 << 20
)

type upstreamRequest struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

type upstreamEvent struct {
	Kind    string `json:"kind"`
	Attempt int    `json:"attempt,omitempty"`
	Status  int    `json:"status,omitempty"`
}

type exchange struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

type fileState struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

type productRun struct {
	Name       string            `json:"name"`
	Binary     string            `json:"binary"`
	Version    string            `json:"version"`
	Commit     string            `json:"commit"`
	SHA256     string            `json:"sha256"`
	Command    []string          `json:"command"`
	ExitStatus int               `json:"exit_status"`
	Stdout     string            `json:"stdout"`
	Stderr     string            `json:"stderr"`
	Client     exchange          `json:"client_exchange"`
	Upstream   []upstreamRequest `json:"upstream_requests"`
	Events     []upstreamEvent   `json:"events"`
	Retries    int               `json:"retry_attempts"`
	Cancelled  bool              `json:"cancelled"`
	Files      []fileState       `json:"durable_files"`
}

type scenarioResult struct {
	Name       string       `json:"name"`
	Comparison []string     `json:"comparison_differences"`
	Runs       []productRun `json:"runs"`
}

type report struct {
	Status          string           `json:"status"`
	ProdexSource    string           `json:"prodex_source"`
	GodexSource     string           `json:"godex_source"`
	MockURL         string           `json:"mock_url"`
	Comparison      []string         `json:"comparison_differences"`
	NegativeControl string           `json:"negative_control"`
	Scenarios       []scenarioResult `json:"scenarios"`
}

type mockPlan struct {
	FirstStatus int
	Delay       time.Duration
}

type runOptions struct {
	retry  bool
	cancel bool
}

type mockServer struct {
	server    *http.Server
	listener  net.Listener
	mu        sync.Mutex
	active    sync.WaitGroup
	requests  []upstreamRequest
	events    []upstreamEvent
	plan      mockPlan
	planStart int
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__differential-codex" {
		os.Exit(runCodexShim(os.Args[2:]))
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || contains(os.Args[1:], "app-server") || contains(os.Args[1:], "exec-server")) {
		os.Exit(runCodexShim(os.Args[1:]))
	}
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-") &&
		(os.Getenv("GODEX_CODEX_BIN") != "" || os.Getenv("PRODEX_CODEX_BIN") != "") {
		os.Exit(runCodexShim(os.Args[1:]))
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("differential", flag.ContinueOnError)
	prodexBin := flags.String("prodex", "", "exact Prodex 0.436.1 product binary")
	godexBin := flags.String("godex", "", "Godex candidate product binary")
	prodexSource := flags.String("prodex-source", "", "exact Prodex 0.436.1 source checkout")
	godexSource := flags.String("godex-source", "", "Godex candidate source checkout")
	expectedGodexCommit := flags.String("godex-commit", "", "expected Godex source HEAD commit SHA")
	scenarioName := flags.String("scenario", "all", "scenario to run: all, success, retry, cancel, or restart")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *prodexBin == "" || *godexBin == "" || *prodexSource == "" || *godexSource == "" || len(*expectedGodexCommit) != 40 {
		return errors.New("usage: differential --prodex BIN --godex BIN --prodex-source DIR --godex-source DIR --godex-commit FULL_SHA")
	}
	prodexSourceCommit, err := gitCommit(*prodexSource)
	if err != nil {
		return fmt.Errorf("inspect Prodex source: %w", err)
	}
	if prodexSourceCommit != prodexCommit {
		return fmt.Errorf("Prodex source commit %s, want exact 0.436.1 commit %s", prodexSourceCommit, prodexCommit)
	}
	godexSourceCommit, err := gitCommit(*godexSource)
	if err != nil {
		return fmt.Errorf("inspect Godex source: %w", err)
	}
	if godexSourceCommit != *expectedGodexCommit {
		return fmt.Errorf("Godex source commit %s, want %s", godexSourceCommit, *expectedGodexCommit)
	}
	root, err := os.MkdirTemp("", "godex-differential-")
	if err != nil {
		return fmt.Errorf("create isolated homes: %w", err)
	}
	defer os.RemoveAll(root)
	mock, err := startMock()
	if err != nil {
		return err
	}
	defer mock.close()

	specs := []struct {
		name string
		run  func() (scenarioResult, error)
	}{
		{"success", func() (scenarioResult, error) {
			return runPair(root, mock, "success", mockPlan{}, *prodexBin, *godexBin, prodexSourceCommit, godexSourceCommit, runOptions{})
		}},
		{"retry", func() (scenarioResult, error) {
			return runPair(root, mock, "retry", mockPlan{FirstStatus: http.StatusTooManyRequests}, *prodexBin, *godexBin, prodexSourceCommit, godexSourceCommit, runOptions{retry: true})
		}},
		{"cancel", func() (scenarioResult, error) {
			return runPair(root, mock, "cancel", mockPlan{Delay: 2 * time.Second}, *prodexBin, *godexBin, prodexSourceCommit, godexSourceCommit, runOptions{cancel: true})
		}},
		{"restart", func() (scenarioResult, error) {
			return runRestartScenario(root, mock, *prodexBin, *godexBin, prodexSourceCommit, godexSourceCommit)
		}},
	}
	var scenarios []scenarioResult
	for _, spec := range specs {
		if *scenarioName != "all" && *scenarioName != spec.name {
			continue
		}
		result, err := spec.run()
		if err != nil {
			return fmt.Errorf("%s scenario: %w", spec.name, err)
		}
		scenarios = append(scenarios, result)
	}
	if len(scenarios) == 0 {
		return fmt.Errorf("unknown scenario %q", *scenarioName)
	}
	var differences []string
	for _, scenario := range scenarios {
		differences = append(differences, scenario.Comparison...)
	}
	differences = uniqueStrings(differences)
	if !negativeControl() {
		return errors.New("negative control failed: status mismatch escaped comparison")
	}
	for _, scenario := range scenarios {
		differences = append(differences, scenarioInvariants(scenario)...)
	}
	differences = uniqueStrings(differences)
	status := "PASS"
	if len(differences) > 0 {
		status = "FAIL"
	}
	result := report{
		Status: status, ProdexSource: prodexSourceCommit, GodexSource: godexSourceCommit,
		MockURL: mock.listener.Addr().String(), Comparison: differences,
		NegativeControl: "PASS: status, headers, events, state, retry, cancellation, and stdout mutations are detected",
		Scenarios:       scenarios,
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if status != "PASS" {
		return fmt.Errorf("differential parity mismatch: %v", differences)
	}
	return nil
}

func runPair(root string, mock *mockServer, scenario string, plan mockPlan, prodexBin, godexBin, prodexCommit, godexCommit string, options runOptions) (scenarioResult, error) {
	scenarioRoot := filepath.Join(root, scenario)
	prodexRun, err := runProduct(scenarioRoot, mock, "prodex", prodexBin, prodexCommit, plan, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Prodex: %w", err)
	}
	godexRun, err := runProduct(scenarioRoot, mock, "godex", godexBin, godexCommit, plan, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Godex: %w", err)
	}
	return scenarioResult{Name: scenario, Comparison: compare(prodexRun, godexRun), Runs: []productRun{prodexRun, godexRun}}, nil
}

func runRestartScenario(root string, mock *mockServer, prodexBin, godexBin, prodexCommit, godexCommit string) (scenarioResult, error) {
	options := runOptions{}
	prodexFirst, err := runProduct(root, mock, "prodex-restart", prodexBin, prodexCommit, mockPlan{}, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Prodex first launch: %w", err)
	}
	prodexSecond, err := runProduct(root, mock, "prodex-restart", prodexBin, prodexCommit, mockPlan{}, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Prodex restart: %w", err)
	}
	godexFirst, err := runProduct(root, mock, "godex-restart", godexBin, godexCommit, mockPlan{}, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Godex first launch: %w", err)
	}
	godexSecond, err := runProduct(root, mock, "godex-restart", godexBin, godexCommit, mockPlan{}, options)
	if err != nil {
		return scenarioResult{}, fmt.Errorf("Godex restart: %w", err)
	}
	differences := compare(prodexFirst, godexFirst)
	differences = append(differences, compare(prodexSecond, godexSecond)...)
	if len(prodexSecond.Upstream) == 0 {
		differences = append(differences, "prodex.restart.requests")
	}
	if len(godexSecond.Upstream) == 0 {
		differences = append(differences, "godex.restart.requests")
	}
	differences = uniqueStrings(differences)
	return scenarioResult{Name: "restart", Comparison: differences, Runs: []productRun{prodexFirst, prodexSecond, godexFirst, godexSecond}}, nil
}

func runProduct(root string, mock *mockServer, name, binary, commit string, plan mockPlan, options runOptions) (productRun, error) {
	resolved, err := filepath.Abs(binary)
	if err != nil {
		return productRun{}, err
	}
	version, err := productVersion(resolved)
	if err != nil {
		return productRun{}, err
	}
	if isProdex(name) && version != "prodex 0.436.1" {
		return productRun{}, fmt.Errorf("binary %s reports %q, want prodex 0.436.1", resolved, version)
	}
	digest, err := fileSHA256(resolved)
	if err != nil {
		return productRun{}, err
	}
	productRoot := filepath.Join(root, name)
	stateHome := filepath.Join(productRoot, "state")
	codexHome := filepath.Join(productRoot, "codex")
	userHome := filepath.Join(productRoot, "user")
	for _, dir := range []string{stateHome, codexHome, userHome, filepath.Join(userHome, ".config")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return productRun{}, err
		}
	}
	childResult := filepath.Join(productRoot, "child-exchange.json")
	if err := os.Remove(childResult); err != nil && !errors.Is(err, os.ErrNotExist) {
		return productRun{}, fmt.Errorf("remove stale child result: %w", err)
	}
	shim, err := os.Executable()
	if err != nil {
		return productRun{}, err
	}
	args := []string{"super", "--provider", "deepseek", "--api-key", apiKey, "--base-url", mock.baseURL() + "/v1", "--no-presidio", "--no-sub-agent", "exec", "synthetic differential probe"}
	stdout, stderr := &limitedBuffer{}, &limitedBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, resolved, args...)
	command.Env = productEnv(name, stateHome, codexHome, userHome, shim, childResult, options)
	command.Stdout, command.Stderr = stdout, stderr
	requestStart, eventStart := mock.configure(plan)
	err = command.Run()
	mock.waitIdle(3 * time.Second)
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
			if ctx.Err() != nil {
				exit = 124
			}
		} else {
			return productRun{}, fmt.Errorf("run %s: %w", name, err)
		}
	}
	clientExchange, readErr := readExchange(childResult)
	if readErr != nil && !options.cancel {
		return productRun{}, fmt.Errorf("%s missing valid child response: %w", name, readErr)
	}
	upstream := mock.since(requestStart)
	if len(upstream) == 0 {
		return productRun{}, fmt.Errorf("%s produced no upstream requests", name)
	}
	events := mock.eventsSince(eventStart)
	return productRun{
		Name: name, Binary: resolved, Version: version, Commit: commit, SHA256: digest,
		Command:    []string{name, "super", "--provider", "deepseek", "--api-key", "<synthetic-key>", "--base-url", "http://127.0.0.1:<mock>/v1", "--no-presidio", "--no-sub-agent", "exec", "synthetic differential probe"},
		ExitStatus: exit, Stdout: redact(stdout.String()), Stderr: redact(stderr.String()),
		Client: clientExchange, Upstream: upstream, Events: events,
		Retries: max(0, len(upstream)-1), Cancelled: options.cancel && hasEvent(events, "upstream.cancelled"), Files: snapshot(productRoot),
	}, nil
}

func productEnv(name, stateHome, codexHome, userHome, shim, childResult string, options runOptions) []string {
	path := os.Getenv("PATH")
	env := []string{
		"PATH=" + path, "HOME=" + userHome, "XDG_CONFIG_HOME=" + filepath.Join(userHome, ".config"),
		"TMPDIR=" + userHome, "TEMP=" + userHome, "TMP=" + userHome, "TERM=dumb",
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"http_proxy=http://127.0.0.1:1", "https_proxy=http://127.0.0.1:1",
		"NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost",
		"DIFFERENTIAL_CHILD_RESULT=" + childResult,
	}
	if options.retry {
		env = append(env, "DIFFERENTIAL_RETRY=1")
	}
	if options.cancel {
		env = append(env, "DIFFERENTIAL_CANCEL=1")
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	if isProdex(name) {
		env = append(env,
			"PRODEX_HOME="+stateHome,
			"PRODEX_SHARED_CODEX_HOME="+codexHome,
			"PRODEX_RUNTIME_LOG_DIR="+filepath.Join(stateHome, "logs"),
			"PRODEX_CODEX_BIN="+shim,
			"PRODEX_PRESIDIO_AUTO_START=0",
		)
	} else {
		env = append(env,
			"GODEX_HOME="+stateHome,
			"CODEX_HOME="+codexHome,
			"GODEX_CODEX_BIN="+shim,
			"GODEX_RUNTIME_LOG_DIR="+filepath.Join(stateHome, "logs"),
		)
	}
	return env
}

func startMock() (*mockServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	mock := &mockServer{listener: listener}
	mock.server = &http.Server{Handler: http.HandlerFunc(mock.serveHTTP), ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = mock.server.Serve(listener) }()
	return mock, nil
}

func (mock *mockServer) baseURL() string { return "http://" + mock.listener.Addr().String() }

func (mock *mockServer) close() { _ = mock.server.Close() }

func (mock *mockServer) waitIdle(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		mock.active.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (mock *mockServer) configure(plan mockPlan) (requestStart, eventStart int) {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	mock.plan = plan
	mock.planStart = len(mock.requests)
	return len(mock.requests), len(mock.events)
}

func (mock *mockServer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	mock.active.Add(1)
	defer mock.active.Done()
	body, err := io.ReadAll(io.LimitReader(request.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		http.Error(writer, "synthetic request rejected", http.StatusRequestEntityTooLarge)
		return
	}
	headers := request.Header.Clone()
	if headers.Get("Authorization") != "" {
		headers.Set("Authorization", "<redacted>")
	}
	mock.mu.Lock()
	attempt := len(mock.requests) - mock.planStart + 1
	mock.requests = append(mock.requests, upstreamRequest{
		Method: request.Method, Path: request.URL.RequestURI(), Headers: headers, Body: string(body),
	})
	mock.events = append(mock.events, upstreamEvent{Kind: "upstream.request", Attempt: attempt})
	plan := mock.plan
	mock.mu.Unlock()
	if request.Method != http.MethodPost {
		http.Error(writer, "synthetic method rejected", http.StatusMethodNotAllowed)
		return
	}
	if plan.Delay > 0 {
		timer := time.NewTimer(plan.Delay)
		select {
		case <-timer.C:
		case <-request.Context().Done():
			timer.Stop()
			mock.mu.Lock()
			mock.events = append(mock.events, upstreamEvent{Kind: "upstream.cancelled", Attempt: attempt})
			mock.mu.Unlock()
			return
		}
	}
	status := plan.FirstStatus
	if status == 0 || attempt != 1 {
		status = http.StatusOK
	}
	responseBody := `{"id":"chatcmpl-differential","object":"chat.completion","created":1,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"synthetic-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	if status != http.StatusOK {
		responseBody = `{"error":{"code":"rate_limit_exceeded"}}`
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Synthetic-Upstream", "differential-v1")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, responseBody)
	mock.mu.Lock()
	mock.events = append(mock.events, upstreamEvent{Kind: "upstream.response", Attempt: attempt, Status: status})
	mock.mu.Unlock()
}

func (mock *mockServer) since(index int) []upstreamRequest {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if index > len(mock.requests) {
		return nil
	}
	return append([]upstreamRequest(nil), mock.requests[index:]...)
}

func (mock *mockServer) eventsSince(index int) []upstreamEvent {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if index > len(mock.events) {
		return nil
	}
	return append([]upstreamEvent(nil), mock.events[index:]...)
}

func runCodexShim(arguments []string) int {
	if contains(arguments, "--version") {
		fmt.Println("codex 0.160.0")
		return 0
	}
	if contains(arguments, "app-server") {
		return runAppServerShim()
	}
	if contains(arguments, "exec-server") {
		return 0
	}
	baseURL := codexBaseURL(arguments)
	if baseURL == "" {
		return shimFailure("synthetic Codex shim could not find a configured provider URL")
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return shimFailure("synthetic Codex shim received an invalid provider URL")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/responses"
	requestBody := []byte(`{"model":"deepseek-v4-pro","input":[{"role":"user","content":[{"type":"input_text","text":"same request"}]}],"stream":false}`)
	timeout := 8 * time.Second
	if os.Getenv("DIFFERENTIAL_CANCEL") == "1" {
		timeout = 1 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	maxAttempts := 1
	if os.Getenv("DIFFERENTIAL_RETRY") == "1" {
		maxAttempts = 2
	}
	var clientResult exchange
	var body []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		request, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(requestBody))
		if err != nil {
			return shimFailure("synthetic Codex shim could not build request")
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Traceparent", "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")
		response, err := client.Do(request)
		if err != nil {
			return shimFailure("synthetic Codex shim request failed: " + err.Error())
		}
		body, err = io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
		response.Body.Close()
		if err != nil || len(body) > bodyLimit {
			return shimFailure("synthetic Codex shim response exceeded its bound")
		}
		clientResult = exchange{Status: response.StatusCode, Headers: response.Header.Clone(), Body: string(body)}
		if response.StatusCode != http.StatusTooManyRequests || attempt+1 == maxAttempts {
			break
		}
	}
	encoded, err := json.Marshal(clientResult)
	if err != nil {
		return shimFailure("synthetic Codex shim could not encode response")
	}
	if resultPath := os.Getenv("DIFFERENTIAL_CHILD_RESULT"); resultPath != "" {
		if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
			return shimFailure("synthetic Codex shim could not record response")
		}
	}
	_, _ = os.Stdout.Write(body)
	_, _ = io.WriteString(os.Stdout, "\n")
	if clientResult.Status < 200 || clientResult.Status >= 300 {
		return 2
	}
	return 0
}

func runAppServerShim() int {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		result := any(map[string]any{})
		if request.Method == "hooks/list" {
			result = map[string]any{"data": []any{}}
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(request.ID), "result": result}); err != nil {
			return 1
		}
	}
	return 0
}

func codexBaseURL(arguments []string) string {
	for _, argument := range arguments {
		_, value, ok := strings.Cut(argument, ".base_url=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, `"'`)
	}
	if home := os.Getenv("CODEX_HOME"); home != "" {
		data, _ := os.ReadFile(filepath.Join(home, "config.toml"))
		for _, line := range strings.Split(string(data), "\n") {
			if index := strings.Index(line, "base_url"); index >= 0 {
				if _, value, ok := strings.Cut(line[index:], "="); ok {
					value = strings.TrimSpace(value)
					if unquoted, err := strconv.Unquote(value); err == nil {
						return unquoted
					}
					return strings.Trim(value, `"'`)
				}
			}
		}
	}
	return ""
}

func shimFailure(message string) int {
	_, _ = fmt.Fprintln(os.Stderr, message)
	return 1
}

// scenarioInvariants guards against equal-but-invalid executions. Matching
// product failures, skipped upstream calls, and missing cancellations must not
// turn into parity successes just because both implementations behave alike.
func scenarioInvariants(scenario scenarioResult) []string {
	var failures []string
	wantRuns := 2
	if scenario.Name == "restart" {
		wantRuns = 4
	}
	if len(scenario.Runs) != wantRuns {
		failures = append(failures, scenario.Name+".run_count")
	}
	for _, run := range scenario.Runs {
		prefix := scenario.Name + "." + run.Name
		wantExit := 0
		if scenario.Name == "cancel" {
			wantExit = 1 // Both canonical clients terminate their interrupted shim.
			if !run.Cancelled {
				failures = append(failures, prefix+".cancellation_missing")
			}
		} else if run.Client.Status != http.StatusOK || run.Client.Body == "" {
			failures = append(failures, prefix+".response_missing")
		}
		if run.ExitStatus != wantExit {
			failures = append(failures, prefix+".exit_status")
		}
		wantRequests := 1
		if scenario.Name == "retry" {
			wantRequests = 2
		}
		if len(run.Upstream) != wantRequests || run.Retries != wantRequests-1 {
			failures = append(failures, prefix+".upstream_attempts")
		}
	}
	return failures
}

func compare(left, right productRun) []string {
	var differences []string
	if left.ExitStatus != right.ExitStatus {
		differences = append(differences, "exit_status")
	}
	if left.Stdout != right.Stdout {
		differences = append(differences, "stdout")
	}
	if left.Stderr != right.Stderr {
		differences = append(differences, "stderr")
	}
	if left.Client.Status != right.Client.Status {
		differences = append(differences, "client.status")
	}
	if !reflect.DeepEqual(left.Client.Headers, right.Client.Headers) {
		differences = append(differences, "client.headers")
	}
	if left.Client.Body != right.Client.Body {
		differences = append(differences, "client.body")
	}
	if !reflect.DeepEqual(left.Upstream, right.Upstream) {
		differences = append(differences, "upstream.requests")
	}
	if !reflect.DeepEqual(left.Events, right.Events) {
		differences = append(differences, "events")
	}
	if left.Retries != right.Retries {
		differences = append(differences, "retry_attempts")
	}
	if left.Cancelled != right.Cancelled {
		differences = append(differences, "cancellation")
	}
	if !reflect.DeepEqual(left.Files, right.Files) {
		differences = append(differences, "durable_files")
	}
	sort.Strings(differences)
	return differences
}

func negativeControl() bool {
	baseline := productRun{ExitStatus: 0, Stdout: "same", Client: exchange{Status: http.StatusOK}, Events: []upstreamEvent{{Kind: "upstream.response", Status: http.StatusOK}}}
	mutations := []struct {
		name   string
		mutate func(productRun) productRun
		want   string
	}{
		{"status", func(run productRun) productRun { run.Client.Status++; return run }, "client.status"},
		{"stdout", func(run productRun) productRun { run.Stdout = "changed"; return run }, "stdout"},
		{"headers", func(run productRun) productRun { run.Client.Headers = http.Header{"X-Mutated": {"1"}}; return run }, "client.headers"},
		{"request", func(run productRun) productRun {
			run.Upstream = []upstreamRequest{{Method: http.MethodGet}}
			return run
		}, "upstream.requests"},
		{"events", func(run productRun) productRun {
			run.Events = append(append([]upstreamEvent(nil), run.Events...), upstreamEvent{Kind: "upstream.extra"})
			return run
		}, "events"},
		{"retry", func(run productRun) productRun { run.Retries++; return run }, "retry_attempts"},
		{"cancellation", func(run productRun) productRun { run.Cancelled = true; return run }, "cancellation"},
		{"state", func(run productRun) productRun { run.Files = []fileState{{Path: "state.json"}}; return run }, "durable_files"},
	}
	for _, mutation := range mutations {
		if !contains(compare(baseline, mutation.mutate(baseline)), mutation.want) {
			return false
		}
	}
	return true
}

func hasEvent(events []upstreamEvent, kind string) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func isProdex(name string) bool { return strings.HasPrefix(name, "prodex") }

func readExchange(path string) (exchange, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return exchange{}, err
	}
	var result exchange
	if err := json.Unmarshal(data, &result); err != nil {
		return exchange{}, err
	}
	return result, nil
}

func snapshot(root string) []fileState {
	var result []fileState
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if entry != nil {
				relative, _ := filepath.Rel(root, path)
				if relative == "user" || strings.HasPrefix(relative, "user"+string(filepath.Separator)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		state := fileState{Path: filepath.ToSlash(relative), Mode: info.Mode().String(), Size: info.Size()}
		lower := strings.ToLower(entry.Name())
		if strings.Contains(lower, "auth") || strings.Contains(lower, "capability") || strings.Contains(lower, "credential") || strings.HasSuffix(lower, ".log") {
			state.Secret = true
		} else if !info.IsDir() {
			state.SHA256, _ = fileSHA256(path)
		}
		result = append(result, state)
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func productVersion(binary string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--version")
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir()}
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitCommit(root string) (string, error) {
	command := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func redact(value string) string { return strings.ReplaceAll(value, apiKey, "<synthetic-key>") }

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type limitedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := bodyLimit - buffer.buffer.Len()
	if remaining <= 0 {
		return len(value), nil
	}
	original := len(value)
	if len(value) > remaining {
		value = value[:remaining]
	}
	_, _ = buffer.buffer.Write(value)
	return original, nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
