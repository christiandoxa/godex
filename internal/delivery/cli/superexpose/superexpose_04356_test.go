package superexpose

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const exposeHelperEnv = "GODEX_SUPER_EXPOSE_HELPER"

func TestMain(m *testing.M) {
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
	}
	os.Exit(m.Run())
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
