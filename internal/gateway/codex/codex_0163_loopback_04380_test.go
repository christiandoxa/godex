//go:build linux && amd64

package codex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type codex0163LoopbackMode uint8

const (
	codex0163LoopbackComplete codex0163LoopbackMode = iota + 1
	codex0163LoopbackIncomplete
	codex0163LoopbackHold
	codex0163LoopbackAutoCompaction
	codex0163LoopbackAsyncCompaction
)

type codex0163LoopbackCall struct {
	path string
	body []byte
}

type codex0163LoopbackProvider struct {
	mode       codex0163LoopbackMode
	server     *httptest.Server
	calls      chan codex0163LoopbackCall
	closed     chan struct{}
	closedOnce sync.Once
	mu         sync.Mutex
	requestLog []codex0163LoopbackCall
}

func newCodex0163LoopbackProvider(mode codex0163LoopbackMode) *codex0163LoopbackProvider {
	provider := &codex0163LoopbackProvider{
		mode:   mode,
		calls:  make(chan codex0163LoopbackCall, 8),
		closed: make(chan struct{}),
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	return provider
}

func (provider *codex0163LoopbackProvider) close() {
	provider.server.Close()
}

func (provider *codex0163LoopbackProvider) handle(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, 16<<20))
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	call := codex0163LoopbackCall{path: request.URL.Path, body: append([]byte(nil), body...)}
	provider.mu.Lock()
	provider.requestLog = append(provider.requestLog, call)
	provider.mu.Unlock()
	provider.calls <- call
	if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	flusher, _ := writer.(http.Flusher)
	if provider.mode == codex0163LoopbackHold {
		writeCodex0163SSE(writer, flusher, "response.created", map[string]any{
			"type": "response.created", "response": map[string]any{"id": "resp-held"},
		})
		select {
		case <-request.Context().Done():
			provider.closedOnce.Do(func() { close(provider.closed) })
		}
		return
	}
	if provider.mode == codex0163LoopbackIncomplete {
		writeCodex0163SSE(writer, flusher, "response.created", map[string]any{
			"type": "response.created", "response": map[string]any{"id": "resp-incomplete"},
		})
		writeCodex0163SSE(writer, flusher, "response.incomplete", map[string]any{
			"type": "response.incomplete",
			"response": map[string]any{
				"id": "resp-incomplete", "status": "incomplete",
				"incomplete_details": map[string]any{"reason": "max_output_tokens"},
				"usage":              map[string]any{"input_tokens": 17, "output_tokens": 5, "total_tokens": 22},
			},
		})
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		return
	}
	callNumber := provider.callCount()
	if provider.mode == codex0163LoopbackAsyncCompaction && provider.handleAsyncCompaction(writer, flusher, callNumber) {
		return
	}
	messageText, totalTokens := "synthetic answer", 9
	if provider.mode == codex0163LoopbackAsyncCompaction && callNumber == 1 {
		totalTokens = 70_000
	}
	if provider.mode == codex0163LoopbackAutoCompaction {
		switch callNumber {
		case 1:
			totalTokens = 70_000
		case 2:
			totalTokens = 330_000
		case 3:
			messageText, totalTokens = "LOCAL_SUMMARY", 200
		case 4:
			totalTokens = 120
		}
	}
	messageID := fmt.Sprintf("msg-fixture-%d", provider.callCount())
	item := map[string]any{
		"type": "message", "id": messageID, "role": "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": messageText, "annotations": []any{}}},
	}
	responseID := fmt.Sprintf("resp-fixture-%d", provider.callCount())
	writeCodex0163SSE(writer, flusher, "response.created", map[string]any{
		"type": "response.created", "response": map[string]any{"id": responseID},
	})
	writeCodex0163SSE(writer, flusher, "response.output_item.added", map[string]any{
		"type": "response.output_item.added", "item": item,
	})
	writeCodex0163SSE(writer, flusher, "response.output_item.done", map[string]any{
		"type": "response.output_item.done", "item": item,
	})
	writeCodex0163SSE(writer, flusher, "response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": responseID, "status": "completed", "output": []any{item},
			"usage": map[string]any{"input_tokens": totalTokens, "output_tokens": 0, "total_tokens": totalTokens},
		},
	})
	_, _ = io.WriteString(writer, "data: [DONE]\n\n")
}

func writeCodex0163SSE(writer http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event, data)
	if flusher != nil {
		flusher.Flush()
	}
}

func (provider *codex0163LoopbackProvider) callCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return len(provider.requestLog)
}

func (provider *codex0163LoopbackProvider) waitCall(ctx context.Context) (codex0163LoopbackCall, error) {
	select {
	case call := <-provider.calls:
		return call, nil
	case <-ctx.Done():
		return codex0163LoopbackCall{}, ctx.Err()
	}
}

func (provider *codex0163LoopbackProvider) assertNoCall(t *testing.T, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case call := <-provider.calls:
		t.Fatalf("unexpected extra loopback request to %s", call.path)
	case <-timer.C:
	}
}

type codex0163RPCMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params map[string]any  `json:"params"`
	Result map[string]any  `json:"result"`
	Error  map[string]any  `json:"error"`
}

type codex0163AppServer struct {
	ctx       context.Context
	cancel    context.CancelFunc
	command   *exec.Cmd
	stdin     io.WriteCloser
	messages  chan codex0163RPCMessage
	readerErr chan error
	mu        sync.Mutex
	status    string
	compacted bool
}

func newCodex0163AppServer(t *testing.T, mode codex0163LoopbackMode) (*codex0163AppServer, *codex0163LoopbackProvider) {
	t.Helper()
	binary := strings.TrimSpace(os.Getenv("GODEX_TEST_CODEX_0163_APP_SERVER_BIN"))
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_0163_APP_SERVER_BIN to the SHA-verified Codex 0.163.0 binary")
	}
	if err := verifyCodex0163Binary04380(binary); err != nil {
		t.Fatal(err)
	}
	provider := newCodex0163LoopbackProvider(mode)
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	if err := os.Mkdir(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("model_provider = %q\n", "loopback")
	if mode == codex0163LoopbackAutoCompaction {
		config += "model_auto_compact_token_limit = 200000\n"
	}
	if mode == codex0163LoopbackAsyncCompaction {
		config += "model_context_window = 100000\nmodel_auto_compact_token_limit = 90000\n[features.async_compaction]\nenabled = true\nmodel_async_compact_threshold_percent = 50\n"
	}
	config += fmt.Sprintf("[model_providers.loopback]\nname = %q\nbase_url = %q\nwire_api = %q\nrequires_openai_auth = false\n", "loopback", provider.server.URL+"/v1", "responses")
	if mode == codex0163LoopbackAsyncCompaction {
		config += "[model_providers.loopback.capabilities]\nremote_compaction = \"v2\"\n"
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	command := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	command.Dir = root
	command.Env = []string{
		"HOME=" + root, "CODEX_HOME=" + codexHome, "PATH=" + os.Getenv("PATH"),
		"NO_COLOR=1", "CI=1", "OPENAI_API_KEY=fixture-not-a-credential",
		"NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost",
	}
	configureCodexAppServerProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		provider.close()
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		provider.close()
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		cancel()
		provider.close()
		t.Fatal(err)
	}
	app := &codex0163AppServer{
		ctx: ctx, cancel: cancel, command: command, stdin: stdin,
		messages: make(chan codex0163RPCMessage, 128), readerErr: make(chan error, 1),
	}
	go app.read(stdout)
	t.Cleanup(func() {
		_ = stdin.Close()
		terminateCodexAppServerProcess(command)
		_ = command.Wait()
		cancel()
		provider.close()
	})
	return app, provider
}

func verifyCodex0163Binary04380(binary string) error {
	file, err := os.Open(binary)
	if err != nil {
		return fmt.Errorf("open official Codex 0.163.0 binary: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 512<<20 {
		_ = file.Close()
		return fmt.Errorf("official Codex artifact is not a bounded regular file: %v", statErr)
	}
	digest := sha256.New()
	_, readErr := io.Copy(digest, file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || hex.EncodeToString(digest.Sum(nil)) != officialCodex0163LinuxSHA04380 {
		return fmt.Errorf("Codex 0.163.0 binary SHA mismatch: read=%v close=%v", readErr, closeErr)
	}
	version, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(version)) != "codex-cli 0.163.0" {
		return fmt.Errorf("unsupported native Codex version=%q err=%v", version, err)
	}
	return nil
}

func (app *codex0163AppServer) read(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	for scanner.Scan() {
		var message codex0163RPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			app.readerErr <- fmt.Errorf("decode Codex app-server message: %w", err)
			return
		}
		if message.Method == "thread/status/changed" {
			if status, ok := message.Params["status"].(map[string]any); ok {
				app.mu.Lock()
				app.status, _ = status["type"].(string)
				app.mu.Unlock()
			}
		}
		if message.Method == "item/started" {
			if item, ok := message.Params["item"].(map[string]any); ok && item["type"] == "contextCompaction" {
				app.mu.Lock()
				app.compacted = true
				app.mu.Unlock()
			}
		}
		app.messages <- message
	}
	if err := scanner.Err(); err != nil {
		app.readerErr <- err
		return
	}
	app.readerErr <- io.EOF
}

func (app *codex0163AppServer) send(id int, method string, params any) error {
	return json.NewEncoder(app.stdin).Encode(map[string]any{"id": id, "method": method, "params": params})
}

func (app *codex0163AppServer) receive() (codex0163RPCMessage, error) {
	select {
	case message := <-app.messages:
		return message, nil
	case err := <-app.readerErr:
		return codex0163RPCMessage{}, err
	case <-app.ctx.Done():
		return codex0163RPCMessage{}, app.ctx.Err()
	}
}

func (app *codex0163AppServer) request(id int, method string, params any) (codex0163RPCMessage, error) {
	if err := app.send(id, method, params); err != nil {
		return codex0163RPCMessage{}, err
	}
	for {
		message, err := app.receive()
		if err != nil {
			return codex0163RPCMessage{}, err
		}
		if string(message.ID) != strconv.Itoa(id) {
			continue
		}
		if len(message.Error) != 0 {
			return message, fmt.Errorf("Codex %s failed: %v", method, message.Error)
		}
		return message, nil
	}
}

func (app *codex0163AppServer) beginOperation() {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.status = ""
	app.compacted = false
}

func (app *codex0163AppServer) waitIdle() error {
	for {
		app.mu.Lock()
		idle := app.status == "idle"
		app.mu.Unlock()
		if idle {
			return nil
		}
		if _, err := app.receive(); err != nil {
			return err
		}
	}
}

func (app *codex0163AppServer) waitIncompleteTurn() (map[string]any, map[string]any, error) {
	var usage map[string]any
	for {
		message, err := app.receive()
		if err != nil {
			return nil, nil, err
		}
		if message.Method == "thread/tokenUsage/updated" {
			usage = message.Params
		}
		if message.Method == "turn/completed" {
			return usage, message.Params, nil
		}
	}
}
