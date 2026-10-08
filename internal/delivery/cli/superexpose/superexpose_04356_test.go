package superexpose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	exposeHelperEnv          = "GODEX_SUPER_EXPOSE_HELPER"
	tunnelHelperEnv          = "GODEX_TUNNEL_CLIENT_HELPER"
	tunnelCaptureEnv         = "GODEX_TUNNEL_CLIENT_CAPTURE"
	tunnelExternalHealthEnv  = "GODEX_TUNNEL_CLIENT_HEALTH_BASE"
	tunnelOversizeVersionEnv = "GODEX_TUNNEL_CLIENT_OVERSIZE_VERSION"
)

func TestMain(m *testing.M) {
	if os.Getenv(tunnelHelperEnv) != "" {
		runTunnelClientTestHelper()
		os.Exit(0)
	}
	switch os.Getenv(exposeHelperEnv) {
	case "success":
		_, _ = os.Stdout.WriteString("hello-expose")
		os.Exit(0)
	case "redact":
		_, _ = os.Stdout.WriteString("Authorization: Bearer sk-1234567890abcdef")
		os.Exit(0)
	case "large":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", execMaxOutputBytes+4096))
		os.Exit(0)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "control-plane":
		_, _ = os.Stdout.WriteString(os.Getenv("CONTROL_PLANE_API_KEY"))
		os.Exit(0)
	case "run-success":
		task, _ := io.ReadAll(os.Stdin)
		_, _ = os.Stdout.WriteString("task=" + string(task) + "\n")
		_, _ = os.Stderr.WriteString("run-stderr\n")
		os.Exit(0)
	case "run-fail":
		_, _ = os.Stderr.WriteString("run-failed\n")
		os.Exit(23)
	case "run-large":
		_, _ = io.Copy(os.Stdout, strings.NewReader(strings.Repeat("y", runOutputMaxBytes+16*1024)))
		os.Exit(0)
	case "run-sleep":
		_, _ = io.ReadAll(os.Stdin)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func isolateExposeOptionalToolDiscovery(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Setenv("GODEX_OPTIMIZERS_HOME", empty)
	t.Setenv("PRODEX_OPTIMIZERS_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", empty)
	t.Setenv("USERPROFILE", empty)
	if goruntime.GOOS == "windows" {
		systemRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
		if systemRoot != "" {
			t.Setenv("PATH", filepath.Join(systemRoot, "System32"))
		}
	} else {
		t.Setenv("PATH", "/usr/bin:/bin")
	}
}

func tunnelClientHelperBinary(t *testing.T) string {
	t.Helper()
	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if goruntime.GOOS != "windows" {
		return sourcePath
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	name := "tunnel-client"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}
	destinationPath := filepath.Join(t.TempDir(), name)
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(destinationPath, 0o700); err != nil {
		t.Fatal(err)
	}
	return destinationPath
}

func runTunnelClientTestHelper() {
	if len(os.Args) < 2 {
		os.Exit(64)
	}
	if os.Args[1] == "--version" {
		sha := strings.Repeat("a", 40)
		_, _ = fmt.Fprintf(os.Stdout, "0.0.15+%s (git sha: %s)\n", sha, sha)
		if os.Getenv(tunnelOversizeVersionEnv) != "" {
			_, _ = io.Copy(os.Stdout, strings.NewReader(strings.Repeat("x", 1024*1024+1)))
		}
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "run" && os.Args[2] == "--help" {
		_, _ = fmt.Fprintln(os.Stdout, "Usage: tunnel-client run --config <path>")
		return
	}
	if os.Args[1] != "run" {
		os.Exit(65)
	}
	valueFor := func(name string) string {
		for index := 1; index+1 < len(os.Args); index++ {
			if os.Args[index] == name {
				return os.Args[index+1]
			}
		}
		return ""
	}
	healthFile := valueFor("--health.url-file")
	if healthFile == "" {
		os.Exit(66)
	}
	if capture := os.Getenv(tunnelCaptureEnv); capture != "" {
		keyPresent := os.Getenv("CONTROL_PLANE_API_KEY") != ""
		_, openAIKeyPresent := os.LookupEnv("OPENAI_API_KEY")
		_, tunnelConfigPresent := os.LookupEnv("TUNNEL_CLIENT_CONFIG")
		_, inheritedTunnelIDPresent := os.LookupEnv("CONTROL_PLANE_TUNNEL_ID")
		content := "args=" + strings.Join(os.Args[1:], "\n") +
			"\nkey_present=" + fmt.Sprint(keyPresent) +
			"\nopenai_api_key_present=" + fmt.Sprint(openAIKeyPresent) +
			"\ntunnel_config_present=" + fmt.Sprint(tunnelConfigPresent) +
			"\ninherited_tunnel_id_present=" + fmt.Sprint(inheritedTunnelIDPresent) + "\n"
		_ = os.WriteFile(capture, []byte(content), 0o600)
	}
	if externalHealth := strings.TrimSpace(os.Getenv(tunnelExternalHealthEnv)); externalHealth != "" {
		if err := os.WriteFile(healthFile, []byte(externalHealth), 0o600); err != nil {
			os.Exit(68)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(67)
	}
	defer listener.Close()
	baseURL := "http://" + listener.Addr().String()
	if err := os.WriteFile(healthFile, []byte(baseURL), 0o600); err != nil {
		os.Exit(68)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/healthz", "/readyz":
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})}
	_ = server.Serve(listener)
}

func TestProdex04356SuperExposeExecArgumentAndBindPolicy(t *testing.T) {
	options, err := parseArguments([]string{"exec", "--listen", "127.0.0.1:4567", "--name", "repo", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Mode != "exec" || options.Listen != "127.0.0.1:4567" || options.Name != "repo" || !options.DryRun {
		t.Fatalf("options = %#v", options)
	}
	if _, err := parseArguments([]string{"exec", "--no-tunnel", "--tunnel"}); err == nil {
		t.Fatal("conflicting tunnel flags unexpectedly accepted")
	}
	var out bytes.Buffer
	if err := Run(t.Context(), []string{"exec", "--listen", "0.0.0.0:9999", "--dry-run"}, &out, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "only binds loopback") {
		t.Fatalf("non-loopback dry-run = %v", err)
	}
	out.Reset()
	if err := Run(t.Context(), []string{"exec", "--listen", "127.0.0.1:0", "--dry-run"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if text := out.String(); !strings.Contains(text, "Godex Super expose dry run") || !strings.Contains(text, "Mode: exec") {
		t.Fatalf("dry-run output = %q", text)
	}
}

func TestProdex04356SuperExposeRecordsLifecycleInGodexRuntimeLog(t *testing.T) {
	isolateExposeOptionalToolDiscovery(t)
	home := t.TempDir()
	t.Setenv("GODEX_HOME", home)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []string{"exec", "--listen", "127.0.0.1:0"}, io.Discard, io.Discard)
	}()
	logPath := filepath.Join(home, "logs", "runtime.jsonl")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(content), "\"kind\":\"super_expose_started\"") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("super expose did not stop after cancellation")
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{
		`"kind":"super_expose_starting"`,
		`"kind":"super_expose_started"`,
		`"mode":"exec"`,
		`"bind":"loopback"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime expose log missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "/mcp/") {
		t.Fatalf("capability path leaked into runtime expose log: %s", text)
	}
}

func TestProdex04356SuperExposeMediaOriginAndNestingPolicies(t *testing.T) {
	if !mcpContentTypeAllowed(" APPLICATION/JSON　; charset=utf-8") {
		t.Fatal("unicode-trimmed application/json content type rejected")
	}
	if mcpContentTypeAllowed("") || mcpContentTypeAllowed("text/plain") {
		t.Fatal("invalid content type accepted")
	}
	if !mcpAcceptAllowed("text/plain;q=0.2,  application/json ; q=1") {
		t.Fatal("application/json accept token rejected")
	}
	if mcpAcceptAllowed("text/plain") {
		t.Fatal("invalid accept accepted")
	}
	if !mcpOriginAllowed("127.0.0.1:9876", "http://127.0.0.1:9876") ||
		!mcpOriginAllowed("localhost", "https://CHATGPT.COM/") ||
		!mcpOriginAllowed("localhost", "https://chat.openai.com/") {
		t.Fatal("trusted origin rejected")
	}
	if mcpOriginAllowed("example.com", " https://example.com/") ||
		mcpOriginAllowed("example.com", "https://example.com/path") {
		t.Fatal("unsafe origin accepted")
	}
	body := []byte(`{"text":"[{}]","items":[{}]}`)
	if !mcpJSONNestingWithinLimit(body, 3) || mcpJSONNestingWithinLimit(body, 2) ||
		mcpJSONNestingWithinLimit([]byte("}"), 64) ||
		mcpJSONNestingWithinLimit([]byte(`{"x":"abc\`), 64) {
		t.Fatal("JSON nesting policy mismatch")
	}
}

func TestProdex04356SuperExposeExecAdvertisesGodexToolAndAcceptsLegacyAlias(t *testing.T) {
	tools := optionalToolSnapshot{tools: []optionalTool{
		{id: "rtk", kind: "Command", available: true, path: os.Args[0], version: "0.50.0"},
		{id: "codebase-memory-mcp", kind: "McpServer"},
	}}
	handler := testExecHandler(t, tools)

	response := performMCP(t, handler, "tools/list", map[string]any{
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d: %s", response.Code, response.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	result := envelope["result"].(map[string]any)
	list := result["tools"].([]any)
	if len(list) != 1 {
		t.Fatalf("tool list = %#v", list)
	}
	tool := list[0].(map[string]any)
	if tool["name"] != godexExecToolName {
		t.Fatalf("advertised tool = %v", tool["name"])
	}
	if strings.Contains(response.Body.String(), legacyExecToolName) {
		t.Fatalf("legacy tool name leaked into canonical advertisement: %s", response.Body.String())
	}
	meta := tool["_meta"].(map[string]any)
	if _, ok := meta["godex/optionalTools"]; !ok {
		t.Fatalf("Godex optional-tool manifest missing: %#v", meta)
	}

	arguments := map[string]any{
		"program": os.Args[0],
		"env":     map[string]any{exposeHelperEnv: "success"},
	}
	call := performMCP(t, handler, "tools/call", map[string]any{
		"name":      legacyExecToolName,
		"arguments": arguments,
		"_meta":     map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, legacyExecToolName, true)
	if call.Code != http.StatusOK || !strings.Contains(call.Body.String(), "hello-expose") {
		t.Fatalf("legacy exec call = %d %s", call.Code, call.Body.String())
	}
}

func TestProdex04356SuperExposeExecMCPFailClosedGates(t *testing.T) {
	handler := testExecHandler(t, optionalToolSnapshot{})

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/wrong", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong capability status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+handler.expectedPath, nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d", response.Code)
	}

	body := rpcBody(1, "ping", map[string]any{
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	})
	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Accept", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content-type status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/plain")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotAcceptable {
		t.Fatalf("accept status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", "https://evil.example/")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("origin status = %d", response.Code)
	}

	missingMetadata := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, strings.NewReader(body))
	missingMetadata.Header.Set("Content-Type", "application/json")
	missingMetadata.Header.Set("Accept", "application/json")
	missingMetadata.Header.Set("MCP-Protocol-Version", mcpCurrentProtocolVersion)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, missingMetadata)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Mcp-Method header mismatch") {
		t.Fatalf("current metadata gate = %d %s", response.Code, response.Body.String())
	}

	unknownArg := performMCP(t, handler, "tools/call", map[string]any{
		"name": godexExecToolName,
		"arguments": map[string]any{
			"program": os.Args[0],
			"env":     map[string]any{exposeHelperEnv: "success"},
			"extra":   true,
		},
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, godexExecToolName, true)
	if unknownArg.Code != http.StatusBadRequest || !strings.Contains(unknownArg.Body.String(), "unknown tool argument: extra") {
		t.Fatalf("unknown argument gate = %d %s", unknownArg.Code, unknownArg.Body.String())
	}
}

func TestProdex04356SuperExposeCurrentNotificationRequiresMetadataBeforeAcceptance(t *testing.T) {
	handler := testExecHandler(t, optionalToolSnapshot{})
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params": map[string]any{
			"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
		},
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("MCP-Protocol-Version", mcpCurrentProtocolVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("notification without method metadata = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("MCP-Protocol-Version", mcpCurrentProtocolVersion)
	request.Header.Set("Mcp-Method", "notifications/initialized")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("valid notification = %d %s", response.Code, response.Body.String())
	}
}

func TestProdex04356SuperExposeDirectExecBoundsTimeoutRedactionAndEnvironment(t *testing.T) {
	tools := optionalToolSnapshot{tools: []optionalTool{
		{id: "rtk", kind: "Command", available: true, path: os.Args[0], version: "0.50.0"},
	}}
	cwd := t.TempDir()
	result, err := executeDirect(t.Context(), map[string]any{
		"program": os.Args[0],
		"cwd":     cwd,
		"env":     map[string]any{exposeHelperEnv: "success"},
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "completed" || result["success"] != true || result["stdout"] != "hello-expose" {
		t.Fatalf("success result = %#v", result)
	}
	if result["cwd"] != cwd || result["arg_count"] != 0 {
		t.Fatalf("success metadata = %#v", result)
	}

	result, err = executeDirect(t.Context(), map[string]any{
		"program": os.Args[0],
		"env": map[string]any{
			exposeHelperEnv:         "control-plane",
			"CONTROL_PLANE_API_KEY": "should-not-leak",
		},
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if result["stdout"] != "" {
		t.Fatalf("control-plane key leaked to child: %#v", result)
	}

	result, err = executeDirect(t.Context(), map[string]any{
		"program": os.Args[0],
		"env":     map[string]any{exposeHelperEnv: "redact"},
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result["stdout"].(string), "sk-1234567890abcdef") ||
		!strings.Contains(result["stdout"].(string), "<redacted>") {
		t.Fatalf("redacted stdout = %#v", result)
	}

	result, err = executeDirect(t.Context(), map[string]any{
		"program": os.Args[0],
		"env":     map[string]any{exposeHelperEnv: "large"},
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if result["stdout_truncated"] != true || len(result["stdout"].(string)) > execMaxOutputBytes {
		t.Fatalf("bounded stdout = len:%d result:%#v", len(result["stdout"].(string)), result)
	}

	result, err = executeDirect(t.Context(), map[string]any{
		"program":    os.Args[0],
		"env":        map[string]any{exposeHelperEnv: "sleep"},
		"timeout_ms": json.Number("50"),
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "timed_out" || result["termination"] != "timeout" || result["success"] != false {
		t.Fatalf("timeout result = %#v", result)
	}

	result, err = executeDirect(t.Context(), map[string]any{
		"program": "optional:rtk",
		"env":     map[string]any{exposeHelperEnv: "success"},
	}, cwd, tools)
	if err != nil {
		t.Fatal(err)
	}
	if result["optional_tool"] != "rtk" || result["stdout"] != "hello-expose" {
		t.Fatalf("optional alias result = %#v", result)
	}

	if _, err := executeDirect(t.Context(), map[string]any{"program": ""}, cwd, tools); err == nil {
		t.Fatal("empty program accepted")
	}
	if _, err := executeDirect(t.Context(), map[string]any{
		"program": os.Args[0],
		"args":    []any{strings.Repeat("x", execMaxArgumentBytes+1)},
	}, cwd, tools); err == nil {
		t.Fatal("oversized argument accepted")
	}
	if _, err := executeDirect(t.Context(), map[string]any{
		"program":    os.Args[0],
		"timeout_ms": json.Number("120001"),
	}, cwd, tools); err == nil {
		t.Fatal("oversized timeout accepted")
	}
}

type exposeAuditCapture struct {
	mu      sync.Mutex
	events  []runtimemodel.Event
	changed chan struct{}
}

func (capture *exposeAuditCapture) Append(_ context.Context, event runtimemodel.Event) error {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.events = append(capture.events, event)
	if capture.changed != nil {
		close(capture.changed)
	}
	capture.changed = make(chan struct{})
	return nil
}

func (capture *exposeAuditCapture) event(kind string) *runtimemodel.Event {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.eventLocked(kind)
}

func (capture *exposeAuditCapture) waitEvent(kind string) *runtimemodel.Event {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		capture.mu.Lock()
		if event := capture.eventLocked(kind); event != nil {
			capture.mu.Unlock()
			return event
		}
		if capture.changed == nil {
			capture.changed = make(chan struct{})
		}
		changed := capture.changed
		capture.mu.Unlock()
		select {
		case <-changed:
		case <-deadline.C:
			return nil
		}
	}
}

func (capture *exposeAuditCapture) eventLocked(kind string) *runtimemodel.Event {
	for index := range capture.events {
		if capture.events[index].Kind != kind {
			continue
		}
		event := capture.events[index]
		if event.Fields != nil {
			fields := make(map[string]string, len(event.Fields))
			for key, value := range event.Fields {
				fields[key] = value
			}
			event.Fields = fields
		}
		return &event
	}
	return nil
}

func (capture *exposeAuditCapture) snapshot() []runtimemodel.Event {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]runtimemodel.Event(nil), capture.events...)
}

func TestProdex04356SuperExposeExecAuditRedactsCommandSecrets(t *testing.T) {
	capture := &exposeAuditCapture{}
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.audit = newExposeAuditLogWithSink(t.Context(), capture)
	secret := "synthetic-super-secret-value"
	otherSecret := "synthetic-token-secret-value"
	response := performMCP(t, handler, "tools/call", map[string]any{
		"name": godexExecToolName,
		"arguments": map[string]any{
			"program": os.Args[0],
			"args":    []any{"--api-key", secret, "--token=" + otherSecret, "visible argument"},
			"env":     map[string]any{exposeHelperEnv: "success"},
		},
		"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcpCurrentProtocolVersion},
	}, godexExecToolName, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "hello-expose") {
		t.Fatalf("audited exec call = %d %s", response.Code, response.Body.String())
	}

	started := capture.event("super_expose_exec_started")
	completed := capture.event("super_expose_exec_completed")
	if started == nil || completed == nil {
		t.Fatalf("exec audit events = %#v", capture.snapshot())
	}
	preview := started.Fields["command"]
	if !strings.Contains(preview, "--api-key <redacted>") ||
		!strings.Contains(preview, "--token=<redacted>") ||
		!strings.Contains(preview, "\"visible argument\"") {
		t.Fatalf("redacted command preview = %q", preview)
	}
	if strings.Contains(preview, secret) || strings.Contains(preview, otherSecret) {
		t.Fatalf("secret leaked in command preview: %q", preview)
	}
	if len(preview) > 2*1024 {
		t.Fatalf("command preview exceeded bound: %d", len(preview))
	}
	if completed.Fields["success"] != "true" || completed.Fields["status"] != "completed" {
		t.Fatalf("completion audit = %#v", completed.Fields)
	}
	rpc := capture.event("super_expose_rpc")
	rpcCompleted := capture.event("super_expose_rpc_completed")
	if rpc == nil || rpcCompleted == nil {
		t.Fatalf("RPC audit events = %#v", capture.snapshot())
	}
	if rpc.Fields["mode"] != "exec" || rpc.Fields["method"] != "tools_call" || rpc.Fields["tool"] != "exec" {
		t.Fatalf("RPC audit = %#v", rpc.Fields)
	}
	if rpcCompleted.Fields["success"] != "true" {
		t.Fatalf("RPC completion audit = %#v", rpcCompleted.Fields)
	}
}

func TestProdex04356SuperExposeTrailingJSONIsAuditedAsParseError(t *testing.T) {
	capture := &exposeAuditCapture{}
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.audit = newExposeAuditLogWithSink(t.Context(), capture)
	request := httptest.NewRequest(http.MethodPost, "http://"+handler.expectedHost+handler.expectedPath,
		strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"} {\"extra\":true}"))
	request.Host = handler.expectedHost
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d body=%s", response.Code, response.Body.String())
	}
	rejected := capture.event("super_expose_rpc_rejected")
	if rejected == nil || rejected.Fields["reason"] != "parse_error" || rejected.Fields["mode"] != "exec" {
		t.Fatalf("trailing JSON audit = %#v", capture.snapshot())
	}
}

func TestProdex04356SuperExposeHTTPRejectionIsAudited(t *testing.T) {
	capture := &exposeAuditCapture{}
	handler := testExecHandler(t, optionalToolSnapshot{})
	handler.audit = newExposeAuditLogWithSink(t.Context(), capture)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp/wrong-capability", strings.NewReader("{}"))
	request.Host = handler.expectedHost
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong capability status = %d", response.Code)
	}
	rejected := capture.event("super_expose_http_rejected")
	if rejected == nil || rejected.Fields["reason"] != "not_found" {
		t.Fatalf("HTTP rejection audit = %#v", capture.snapshot())
	}
}

func TestProdex04356SuperExposeRateLimitIs120PerSecond(t *testing.T) {
	var limiter rateLimiter
	now := time.Unix(1_700_000_000, 0)
	for index := 0; index < 120; index++ {
		if !limiter.admit(now) {
			t.Fatalf("request %d was rejected early", index)
		}
	}
	if limiter.admit(now) {
		t.Fatal("121st request was admitted")
	}
	if !limiter.admit(now.Add(time.Second)) {
		t.Fatal("rate limit did not reset after one second")
	}
}

func testExecHandler(t *testing.T, tools optionalToolSnapshot) *execMCPHandler {
	t.Helper()
	return &execMCPHandler{
		expectedPath:  "/mcp/test-capability",
		expectedHost:  "127.0.0.1:9876",
		displayName:   "repo",
		instanceID:    "gdxi_test",
		workspace:     t.TempDir(),
		mode:          "exec",
		optionalTools: tools,
	}
}

func performMCP(t *testing.T, handler *execMCPHandler, method string, params map[string]any, name string, current bool) *httptest.ResponseRecorder {
	t.Helper()
	body := rpcBody(1, method, params)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+handler.expectedPath, strings.NewReader(body))
	request.Host = handler.expectedHost
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if current {
		request.Header.Set("MCP-Protocol-Version", mcpCurrentProtocolVersion)
		request.Header.Set("Mcp-Method", method)
		if name != "" {
			request.Header.Set("Mcp-Name", name)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func rpcBody(id any, method string, params map[string]any) string {
	value := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestProdex04356SuperExposeInstanceAndCapabilityShapesAreGodexNative(t *testing.T) {
	token, err := capabilityToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 30 || strings.ContainsAny(token, "+/=") {
		t.Fatalf("capability token = %q", token)
	}
	instance, err := exposeInstanceID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(instance, "gdxi_") {
		t.Fatalf("instance id = %q", instance)
	}
	if filepath.Separator == 0 {
		t.Fatal("unreachable")
	}
}

func TestProdex04356SuperExposeTunnelLifecycleIsAudited(t *testing.T) {
	isolateExposeOptionalToolDiscovery(t)
	home := t.TempDir()
	t.Setenv("GODEX_HOME", home)
	t.Setenv(tunnelHelperEnv, "1")
	helper := tunnelClientHelperBinary(t)
	t.Setenv("CONTROL_PLANE_API_KEY", "synthetic-control-key")
	validID := "tunnel_" + strings.Repeat("f", 32)
	options, err := parseArguments([]string{
		"exec", "--listen", "127.0.0.1:0", "--openai-tunnel-id", validID,
	})
	if err != nil {
		t.Fatal(err)
	}
	starter := func(endpoint, tunnelID string) (*openAITunnelProcess, error) {
		return spawnOpenAITunnel(endpoint, tunnelID, helper, "0.0.15")
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runExecServerWithTunnelStarter(ctx, options, io.Discard, io.Discard, starter)
	}()

	logPath := filepath.Join(home, "logs", "runtime.jsonl")
	deadline := time.Now().Add(openAITunnelReadyTimeout)
	var text string
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(logPath)
		if err == nil {
			text = string(content)
			if strings.Contains(text, `"kind":"super_expose_openai_tunnel_ready"`) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tunneled expose did not stop")
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text = string(content)
	for _, want := range []string{
		`"kind":"super_expose_openai_tunnel_starting"`,
		`"kind":"super_expose_openai_tunnel_ready"`,
		`"provider":"openai"`,
		`"client_version":"0.0.15"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tunnel audit missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, validID) || strings.Contains(text, "synthetic-control-key") || strings.Contains(text, "/mcp/") {
		t.Fatalf("tunnel audit leaked sensitive endpoint material: %s", text)
	}
}

func TestProdex04356OpenAITunnelBinaryPrefersGodexOverride(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_TUNNEL_CLIENT_BIN", binary)
	t.Setenv("PRODEX_TUNNEL_CLIENT_BIN", filepath.Join(t.TempDir(), "must-not-win"))
	got, err := openAITunnelBinary()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(binary) {
		t.Fatalf("tunnel binary = %q, want %q", got, binary)
	}
}

func TestProdex04356OpenAITunnelIDAndClientVersionPolicy(t *testing.T) {
	validID := "tunnel_" + strings.Repeat("a", 32)
	if got, err := validateOpenAITunnelID(validID); err != nil || got != validID {
		t.Fatalf("valid tunnel id = %q, %v", got, err)
	}
	if got, err := validateOpenAITunnelID("  " + validID + "  "); err != nil || got != validID {
		t.Fatalf("trimmed tunnel id = %q, %v", got, err)
	}
	for _, value := range []string{
		"tunnel_short",
		"tunnel_" + strings.Repeat("A", 32),
		"other_" + strings.Repeat("a", 32),
	} {
		if _, err := validateOpenAITunnelID(value); err == nil {
			t.Fatalf("invalid tunnel id accepted: %q", value)
		}
	}
	sha := strings.Repeat("1", 40)
	for _, value := range []struct {
		text string
		want string
		ok   bool
	}{
		{"0.0.13+" + sha + " (git sha: " + sha + ")", "0.0.13", true},
		{"0.0.15+" + sha + " (git sha: " + sha + ")", "0.0.15", true},
		{"0.0.16+" + sha + " (git sha: " + sha + ")", "0.0.16", true},
		{"0.0.12+" + sha + " (git sha: " + sha + ")", "", false},
		{"0.0.15-rc.1+" + sha + " (git sha: " + sha + ")", "", false},
		{"0..15+" + sha + " (git sha: " + sha + ")", "", false},
		{"00.0.15+" + sha + " (git sha: " + sha + ")", "", false},
		{"0.00.15+" + sha + " (git sha: " + sha + ")", "", false},
		{"0.0.015+" + sha + " (git sha: " + sha + ")", "", false},
		{"0.0.15", "", false},
	} {
		got, ok := supportedTunnelClientVersion(value.text)
		if ok != value.ok || got != value.want {
			t.Fatalf("version %q = %q,%t; want %q,%t", value.text, got, ok, value.want, value.ok)
		}
	}
}

func TestProdex04356OpenAITunnelCredentialsDistinguishMissingAndInvalid(t *testing.T) {
	if err := os.Unsetenv("CONTROL_PLANE_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := openAITunnelAPIKeyFromEnv(); err == nil ||
		!strings.Contains(err.Error(), "requires CONTROL_PLANE_API_KEY in noninteractive mode") {
		t.Fatalf("missing API key error = %v", err)
	}

	for _, invalid := range []string{"", "bad\nkey", "bad\u0085key"} {
		t.Setenv("CONTROL_PLANE_API_KEY", invalid)
		if _, err := openAITunnelAPIKeyFromEnv(); err == nil ||
			!strings.Contains(err.Error(), "OpenAI Secure MCP Tunnel API key is invalid") {
			t.Fatalf("invalid API key %q error = %v", invalid, err)
		}
	}

	if goruntime.GOOS != "windows" {
		t.Setenv("CONTROL_PLANE_API_KEY", string([]byte{0xff}))
		if _, err := openAITunnelAPIKeyFromEnv(); err == nil ||
			!strings.Contains(err.Error(), "requires CONTROL_PLANE_API_KEY in noninteractive mode") {
			t.Fatalf("non-UTF8 API key error = %v", err)
		}
	}
}

func TestProdex04356OpenAITunnelURLsRejectUnicodeWhitespaceAndInvalidUTF8(t *testing.T) {
	for _, value := range []string{
		"http://127.0.0.1:4567/mcp/ capability",
		"http://127.0.0.1:4567/mcp/capability",
	} {
		if _, err := validateLocalTunnelMCPURL(value); err == nil {
			t.Fatalf("local MCP URL accepted Unicode whitespace/control: %q", value)
		}
	}

	for _, value := range []string{
		"http://127.0.0.1: 4567",
		"http://127.0.0.1:4567",
	} {
		if _, err := validateTunnelHealthBase(value); err == nil {
			t.Fatalf("health URL accepted Unicode whitespace/control: %q", value)
		}
	}

	healthFile := filepath.Join(t.TempDir(), "health-url")
	if err := os.WriteFile(healthFile, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readTunnelHealthBase(healthFile); err == nil ||
		!strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 health URL error = %v", err)
	}
}

func TestProdex04356OpenAITunnelHealthBaseTrimsAllTrailingSlashes(t *testing.T) {
	got, err := validateTunnelHealthBase("  http://127.0.0.1:4567///  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:4567" {
		t.Fatalf("health base = %q", got)
	}
}

func TestProdex04356OpenAITunnelProbeRejectsOversizeVersionOutput(t *testing.T) {
	t.Setenv(tunnelHelperEnv, "1")
	t.Setenv(tunnelOversizeVersionEnv, "1")
	t.Setenv("GODEX_TUNNEL_CLIENT_BIN", tunnelClientHelperBinary(t))
	if _, _, err := ensureOpenAITunnelAvailable(); err == nil {
		t.Fatal("oversize tunnel-client version output was accepted")
	}
}

func TestProdex04356OpenAITunnelPrivateFilesRetryCollision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	nextTunnelConfigID.Store(700)
	collision := filepath.Join(root, fmt.Sprintf("godex-openai-tunnel-%d-701-0", os.Getpid()))
	if err := os.Mkdir(collision, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := createOpenAITunnelFiles("http://127.0.0.1:4567/mcp/capability")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if directory == collision {
		t.Fatalf("private tunnel directory reused colliding path %q", directory)
	}
	if !strings.HasSuffix(directory, "-701-1") {
		t.Fatalf("collision retry directory = %q", directory)
	}
}

func TestProdex04356OpenAITunnelReadinessRejectsChildExitAfterHealth(t *testing.T) {
	validID := "tunnel_" + strings.Repeat("e", 32)
	t.Setenv(tunnelHelperEnv, "1")
	helper := tunnelClientHelperBinary(t)
	t.Setenv("CONTROL_PLANE_API_KEY", "synthetic-control-key")

	var tunnel *openAITunnelProcess
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/healthz":
			writer.WriteHeader(http.StatusOK)
		case "/readyz":
			if tunnel == nil || tunnel.command == nil || tunnel.command.Process == nil {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = tunnel.command.Process.Kill()
			time.Sleep(50 * time.Millisecond)
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(tunnelExternalHealthEnv, server.URL)

	var err error
	tunnel, err = spawnOpenAITunnel("http://127.0.0.1:4567/mcp/capability", validID, helper, "0.0.15")
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.shutdown()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := tunnel.waitReady(ctx); err == nil || !strings.Contains(err.Error(), "exited before local readiness") {
		t.Fatalf("readiness after child exit = %v", err)
	}
}

func TestProdex04356OpenAITunnelDryRunValidatesWithoutClientProbe(t *testing.T) {
	validID := "tunnel_" + strings.Repeat("b", 32)
	t.Setenv("GODEX_TUNNEL_CLIENT_BIN", filepath.Join(t.TempDir(), "missing-tunnel-client"))
	var out bytes.Buffer
	if err := Run(t.Context(), []string{
		"exec", "--openai-tunnel-id", validID, "--dry-run",
	}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Tunnel: OpenAI Secure MCP Tunnel "+validID) {
		t.Fatalf("dry-run tunnel output = %q", out.String())
	}
	if err := Run(t.Context(), []string{
		"exec", "--openai-tunnel-id", "tunnel_short", "--dry-run",
	}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "OpenAI tunnel id") {
		t.Fatalf("invalid tunnel dry-run = %v", err)
	}
	if err := Run(t.Context(), []string{
		"exec", "--tunnel", "--dry-run",
	}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "legacy --tunnel mode") {
		t.Fatalf("legacy tunnel = %v", err)
	}
}

func TestProdex04356OpenAITunnelStartsOfficialClientWithPrivateConfigAndHealth(t *testing.T) {
	validID := "tunnel_" + strings.Repeat("c", 32)
	capture := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv(tunnelHelperEnv, "1")
	t.Setenv(tunnelCaptureEnv, capture)
	helper := tunnelClientHelperBinary(t)
	t.Setenv("CONTROL_PLANE_API_KEY", "synthetic-control-key")
	t.Setenv("OPENAI_API_KEY", "must-not-inherit")
	t.Setenv("TUNNEL_CLIENT_CONFIG", "must-not-inherit")

	tunnel, err := spawnOpenAITunnel("http://127.0.0.1:4567/mcp/capability", validID, helper, "0.0.15")
	if err != nil {
		t.Fatal(err)
	}
	directory := tunnel.directory
	defer tunnel.shutdown()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := tunnel.waitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if tunnel.version != "0.0.15" || tunnel.id != validID {
		t.Fatalf("tunnel status = id:%q version:%q", tunnel.id, tunnel.version)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 && goruntime.GOOS != "windows" {
		t.Fatalf("tunnel directory mode = %o", info.Mode().Perm())
	}
	for _, name := range []string{"config.yaml", "mcp-url", "health-url"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 && goruntime.GOOS != "windows" {
			t.Fatalf("%s mode = %o", name, info.Mode().Perm())
		}
	}
	config, err := os.ReadFile(filepath.Join(directory, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "config_version: 1") ||
		!strings.Contains(string(config), "file:"+filepath.Join(directory, "mcp-url")) {
		t.Fatalf("tunnel config = %q", config)
	}
	captured, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	text := string(captured)
	for _, want := range []string{
		"run", "--control-plane.base-url", "https://api.openai.com",
		"--control-plane.tunnel-id", validID,
		"--control-plane.api-key", "env:CONTROL_PLANE_API_KEY",
		"--health.listen-addr", "127.0.0.1:0",
		"key_present=true",
		"openai_api_key_present=false",
		"tunnel_config_present=false",
		"inherited_tunnel_id_present=false",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tunnel argv capture missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "synthetic-control-key") || strings.Contains(text, "must-not-inherit") {
		t.Fatalf("tunnel capture leaked secret/inherited config: %s", text)
	}

	tunnel.shutdown()
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tunnel private directory survived cleanup: %v", err)
	}
}
