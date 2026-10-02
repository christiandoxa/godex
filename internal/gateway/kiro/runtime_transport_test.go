package kiro

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestKiroRuntimeTransportBufferedAndSynthesizedStreaming(t *testing.T) {
	transport := newFakeKiroRuntimeTransport(t, "hello from ACP", "thinking")
	assertKiroResponsesRoute(t, transport)
	assertKiroBufferedCompatRoutes(t, transport)
	assertKiroSynthesizedResponsesStream(t, transport)
}

func newFakeKiroRuntimeTransport(t *testing.T, text, reasoning string) *RuntimeTransport {
	t.Helper()
	home := writeKiroRuntimeHome(t)
	source := NewSource()
	source.acp = func(_ context.Context, _, model, _, _ string) (acpTurn, error) {
		return fixtureACPTurn(model, text, reasoning), nil
	}
	transport, err := source.NewRuntimeTransport(context.Background(), home, "kiro-work")
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func assertKiroResponsesRoute(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	responses, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: runtimeMountPath + "/responses",
		Body: []byte(`{"model":"luna","reasoning":{"effort":"medium"},"input":"hello Kiro"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(responses.Body)
	responses.Body.Close()
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["model"] != "gpt-5.6-luna" || response["requested_model"] != "gpt-5.6-luna" || kiroResponseText(response) != "hello from ACP" {
		t.Fatalf("Responses body = %#v", response)
	}
	metadata := response["metadata"].(map[string]any)["kiro"].(map[string]any)
	if metadata["profile_name"] != "kiro-work" || metadata["reasoning_content"] != "thinking" {
		t.Fatalf("Responses metadata = %#v", metadata)
	}
}

func assertKiroBufferedCompatRoutes(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	fixtures := []struct{ path, body, want string }{
		{runtimeMountPath + "/v1/chat/completions", `{"model":"gpt-5.6-luna","messages":[{"role":"user","content":"hello Kiro"}]}`, `"object":"chat.completion"`},
		{runtimeMountPath + "/messages", `{"model":"gpt-5.6-luna","max_tokens":1024,"messages":[{"role":"user","content":"hello Kiro"}]}`, `"type":"message"`},
	}
	for _, fixture := range fixtures {
		got, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: fixture.path, Body: []byte(fixture.body)}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(got.Body)
		got.Body.Close()
		if !strings.Contains(string(content), fixture.want) || !strings.Contains(string(content), "hello from ACP") {
			t.Fatalf("route %s body = %s", fixture.path, content)
		}
	}
}

func assertKiroSynthesizedResponsesStream(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	streamed, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: runtimeMountPath + "/responses",
		Body: []byte(`{"model":"luna","input":"hello Kiro","stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	streamBody, _ := io.ReadAll(streamed.Body)
	streamed.Body.Close()
	if streamed.Header.Get("Content-Type") != "text/event-stream" || !strings.Contains(string(streamBody), "response.output_text.delta") || !strings.Contains(string(streamBody), "response.completed") {
		t.Fatalf("Synthesized Responses SSE = headers:%v body:%s", streamed.Header, streamBody)
	}
}

func TestKiroRuntimeModelsCompactAndRoutePolicy(t *testing.T) {
	home := writeKiroRuntimeHome(t)
	if err := os.WriteFile(filepath.Join(home, ModelCatalogFile), []byte(`{"models":[{"id":"account-model","name":"Account Model","context_window_tokens":64000}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source := NewSource()
	source.acp = func(_ context.Context, _, _, _, _ string) (acpTurn, error) {
		return fixtureACPTurn("gpt-5.6-luna", "semantic summary", ""), nil
	}
	transport, err := source.NewRuntimeTransport(context.Background(), home, "kiro-work")
	if err != nil {
		t.Fatal(err)
	}
	assertKiroModelsRoute(t, transport)
	assertKiroCompactRoute(t, transport)
	assertKiroUnsupportedRoutes(t, transport)
}

func assertKiroModelsRoute(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	models, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: runtimeMountPath + "/v1/models"}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(models.Body)
	models.Body.Close()
	for _, expected := range []string{"gpt-5.6-luna", "auto", "account-model"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("Models missing %q: %s", expected, body)
		}
	}
}

func assertKiroCompactRoute(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	compact, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: runtimeMountPath + "/responses/compact",
		Body: []byte(`{"model":"auto","input":[{"type":"message","role":"user","content":"summarize this"}]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(compact.Body)
	compact.Body.Close()
	if compact.Header.Get("X-Prodex-Compact-Mode") != "semantic" || compact.Header.Get("X-Prodex-Compact-Provider") != "kiro" || !strings.Contains(string(body), "semantic summary") {
		t.Fatalf("Compact = headers:%v body:%s", compact.Header, body)
	}
}

func assertKiroUnsupportedRoutes(t *testing.T, transport *RuntimeTransport) {
	t.Helper()
	badVersion := "/v" + "2/responses"
	for _, path := range []string{runtimeMountPath + badVersion, runtimeMountPath + "/v1.2/responses", runtimeMountPath + "/v1/responses/", runtimeMountPath + "/v1//responses"} {
		got, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: path, Body: []byte(`{"input":"x"}`)}, proxymodel.Account{})
		if err != nil {
			t.Fatalf("unsupported route %q transport error: %v", path, err)
		}
		if got.StatusCode != http.StatusNotFound {
			t.Fatalf("unsupported route %q status = %d", path, got.StatusCode)
		}
		got.Body.Close()
	}
}

func fixtureACPTurn(model, text, reasoning string) acpTurn {
	notifications := []acpEnvelope{{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(`{"sessionId":"sess_1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"` + text + `"}}}`)}}
	if reasoning != "" {
		notifications = append(notifications, acpEnvelope{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(`{"sessionId":"sess_1","update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"` + reasoning + `"}}}`)})
	}
	return acpTurn{
		Session:       acpSession{SessionID: "sess_1", Models: &acpModelState{CurrentModelID: model}},
		Prompt:        acpEnvelope{JSONRPC: "2.0", ID: json.RawMessage(`2`), Result: json.RawMessage(`{"stopReason":"end_turn"}`)},
		Notifications: notifications,
	}
}

func writeKiroRuntimeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	content := `{"auth_key":"kirocli:social:token","auth_kind":"social","auth_json":"{}","email":"person@example.test"}`
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestKiroRuntimeValidationErrorsExposeReferenceCodes(t *testing.T) {
	transport := newFakeKiroRuntimeTransport(t, "unused", "")
	fixtures := []struct {
		name    string
		path    string
		body    string
		code    string
		message string
	}{
		{
			name:    "chat parallel tool calls",
			path:    runtimeMountPath + "/chat/completions",
			body:    `{"messages":[{"role":"user","content":"hello"}],"parallel_tool_calls":false}`,
			code:    "unsupported_parallel_tool_calls",
			message: "parallel_tool_calls",
		},
		{
			name:    "non object responses body",
			path:    runtimeMountPath + "/responses",
			body:    `[]`,
			code:    "invalid_request_body",
			message: "must be a JSON object",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			response, err := transport.Execute(context.Background(), proxymodel.Request{
				Method: http.MethodPost, Path: fixture.path, Body: []byte(fixture.body),
			}, proxymodel.Account{})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatalf("decode validation error: %v; body=%s", err, body)
			}
			errorObject := value["error"].(map[string]any)
			if response.StatusCode != http.StatusBadRequest || errorObject["code"] != fixture.code || errorObject["type"] != "invalid_request_error" || !strings.Contains(errorObject["message"].(string), fixture.message) {
				t.Fatalf("validation response = status:%d body:%#v", response.StatusCode, value)
			}
		})
	}
}
