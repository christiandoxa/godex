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
	Files      []fileState       `json:"durable_files"`
}

type report struct {
	Status          string       `json:"status"`
	ProdexSource    string       `json:"prodex_source"`
	GodexSource     string       `json:"godex_source"`
	Scenario        string       `json:"scenario"`
	MockURL         string       `json:"mock_url"`
	Comparison      []string     `json:"comparison_differences"`
	NegativeControl string       `json:"negative_control"`
	Runs            []productRun `json:"runs"`
}

type mockServer struct {
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	requests []upstreamRequest
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
	godexBin := flags.String("godex", "", "Godex baseline product binary")
	prodexSource := flags.String("prodex-source", "", "exact Prodex 0.436.1 source checkout")
	godexSource := flags.String("godex-source", "", "Godex source checkout")
	godexExpectedCommit := flags.String("godex-commit", "", "expected Godex source commit SHA (full 40 hexadecimal characters)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *prodexBin == "" || *godexBin == "" || *prodexSource == "" || *godexSource == "" || len(*godexExpectedCommit) != 40 {
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
	if godexSourceCommit != *godexExpectedCommit {
		return fmt.Errorf("Godex source commit %s, want exact expected %s", godexSourceCommit, *godexExpectedCommit)
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

	prodexRun, err := runProduct(root, mock, "prodex", *prodexBin, prodexSourceCommit)
	if err != nil {
		return fmt.Errorf("Prodex scenario: %w", err)
	}
	godexRun, err := runProduct(root, mock, "godex", *godexBin, godexSourceCommit)
	if err != nil {
		return fmt.Errorf("Godex scenario: %w", err)
	}
	status, differences := parityStatus(prodexRun, godexRun)
	if !negativeControl() {
		return errors.New("negative control failed: status mismatch escaped comparison")
	}
	result := report{
		Status: status, ProdexSource: prodexSourceCommit, GodexSource: godexSourceCommit,
		Scenario: "DeepSeek Responses request through each product's local gateway to one deterministic OpenAI Chat Completions mock",
		MockURL:  mock.listener.Addr().String(), Comparison: differences,
		NegativeControl: "PASS: a synthetic client status change is reported as client.status",
		Runs:            []productRun{prodexRun, godexRun},
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if status != "PASS" {
		return fmt.Errorf("differential mismatch: reference exit=%d, Godex exit=%d, fields=%v", prodexRun.ExitStatus, godexRun.ExitStatus, differences)
	}
	return nil
}

func runProduct(root string, mock *mockServer, name, binary, commit string) (productRun, error) {
	resolved, err := filepath.Abs(binary)
	if err != nil {
		return productRun{}, err
	}
	version, err := productVersion(resolved)
	if err != nil {
		return productRun{}, err
	}
	if name == "prodex" && version != "prodex 0.436.1" {
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
	shim, err := os.Executable()
	if err != nil {
		return productRun{}, err
	}
	args := []string{"super", "--provider", "deepseek", "--api-key", apiKey, "--base-url", mock.baseURL() + "/v1", "--no-presidio", "--no-sub-agent", "exec", "synthetic differential probe"}
	stdout, stderr := &limitedBuffer{}, &limitedBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, resolved, args...)
	command.Env = productEnv(name, stateHome, codexHome, userHome, shim, childResult)
	command.Stdout, command.Stderr = stdout, stderr
	start := mock.count()
	err = command.Run()
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
	clientExchange, err := readExchange(childResult)
	if err != nil {
		return productRun{}, fmt.Errorf("%s produced no valid child response evidence: %w", name, err)
	}
	if mock.count() == start {
		return productRun{}, fmt.Errorf("%s produced no upstream request evidence", name)
	}
	return productRun{
		Name: name, Binary: resolved, Version: version, Commit: commit, SHA256: digest,
		Command:    []string{name, "super", "--provider", "deepseek", "--api-key", "<synthetic-key>", "--base-url", "http://127.0.0.1:<mock>/v1", "--no-presidio", "--no-sub-agent", "exec", "synthetic differential probe"},
		ExitStatus: exit, Stdout: redact(stdout.String()), Stderr: redact(stderr.String()),
		Client: clientExchange, Upstream: mock.since(start), Files: snapshot(productRoot),
	}, nil
}

func productEnv(name, stateHome, codexHome, userHome, shim, childResult string) []string {
	path := os.Getenv("PATH")
	env := []string{
		"PATH=" + path, "HOME=" + userHome, "XDG_CONFIG_HOME=" + filepath.Join(userHome, ".config"),
		"TMPDIR=" + userHome, "TEMP=" + userHome, "TMP=" + userHome, "TERM=dumb",
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"http_proxy=http://127.0.0.1:1", "https_proxy=http://127.0.0.1:1",
		"NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost",
		"DIFFERENTIAL_CHILD_RESULT=" + childResult,
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	if name == "prodex" {
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

func (mock *mockServer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
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
	mock.requests = append(mock.requests, upstreamRequest{
		Method: request.Method, Path: request.URL.RequestURI(), Headers: headers, Body: string(body),
	})
	mock.mu.Unlock()
	if request.Method != http.MethodPost {
		http.Error(writer, "synthetic method rejected", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Synthetic-Upstream", "differential-v1")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, `{"id":"chatcmpl-differential","object":"chat.completion","created":1,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"synthetic-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
}

func (mock *mockServer) count() int {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	return len(mock.requests)
}

func (mock *mockServer) since(index int) []upstreamRequest {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if index > len(mock.requests) {
		return nil
	}
	return append([]upstreamRequest(nil), mock.requests[index:]...)
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
	request, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(requestBody))
	if err != nil {
		return shimFailure("synthetic Codex shim could not build request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Traceparent", "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01")
	client := &http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return shimFailure("synthetic Codex shim request failed: " + err.Error())
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		return shimFailure("synthetic Codex shim response exceeded its bound")
	}
	clientResult := exchange{Status: response.StatusCode, Headers: response.Header.Clone(), Body: string(body)}
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
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

func parityStatus(left, right productRun) (string, []string) {
	differences := compare(left, right)
	if len(differences) != 0 || left.ExitStatus != 0 || right.ExitStatus != 0 {
		return "FAIL", differences
	}
	return "PASS", differences
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
	if !reflect.DeepEqual(left.Files, right.Files) {
		differences = append(differences, "durable_files")
	}
	sort.Strings(differences)
	return differences
}

func negativeControl() bool {
	baseline := productRun{ExitStatus: 0, Client: exchange{Status: http.StatusOK}}
	mutated := baseline
	mutated.Client.Status++
	return contains(compare(baseline, mutated), "client.status")
}

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
		if walkErr != nil || entry.IsDir() {
			if entry.IsDir() {
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
	if len(value) > remaining {
		value = value[:remaining]
	}
	_, _ = buffer.buffer.Write(value)
	return len(value), nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
