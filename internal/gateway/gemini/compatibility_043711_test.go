package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex043711GeminiAPIKeyPoolUsesOpenAICompatibleChatWire(t *testing.T) {
	var captured map[string]any
	var path, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, authorization = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode native Gemini API-key fixture: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": "synthetic-ok"}}},
				"finishReason": "STOP",
			}},
			"modelVersion":  "gemini-3.1-pro-preview",
			"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3},
		})
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "synthetic-credential"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	body := []byte(`{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":[{"type":"input_text","text":"same request"}]}],"stream":false}`)
	reply, err := pool.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: body,
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer reply.Body.Close()
	content, err := io.ReadAll(reply.Body)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1beta/openai/chat/completions" || authorization != "Bearer synthetic-credential" {
		t.Fatalf("tagged Gemini API-key upstream=%q authorization configured=%t", path, authorization != "")
	}
	messages, _ := captured["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" ||
		messages[0].(map[string]any)["content"] != "same request" {
		t.Fatalf("API-key chat request was not translated: %+v", captured)
	}
	if reply.StatusCode != http.StatusOK || !strings.Contains(string(content), "synthetic-ok") ||
		!strings.Contains(string(content), `"object":"response"`) {
		t.Fatalf("API-key chat response was not converted to Responses: status=%d body=%s", reply.StatusCode, content)
	}
}

func TestProdex043711GeminiAPIKeyPoolDropsDanglingToolCallWithoutOutput(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"id\":\"chatcmpl-gemini\",\"object\":\"chat.completion\",\"model\":\"gemini-3.1-pro-preview\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}")
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta", []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "synthetic-credential"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	request := []byte(`{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":[{"type":"input_text","text":"same request"}]},{"type":"function_call","call_id":"call-fixture","name":"lookup","arguments":"{}"}],"stream":false}`)
	result, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: request,
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	messages, _ := captured["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
		t.Fatalf("canonical Gemini chat request includes orphan tool call: %+v", captured)
	}
}

func TestProdex043711GeminiAPIKeyPoolChatCompletionBodyFailsLikeOfficialBinary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" {
			t.Errorf("wrong Gemini route: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-gemini", "object": "chat.completion",
			"model": "gemini-3.1-pro-preview",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "synthetic-ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3},
		})
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL+"/v1beta",
		[]proxymodel.ProviderCredential{{ID: "key-a", Secret: "synthetic-credential"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	result, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses",
		Body: []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","stream":false}`),
	}, proxymodel.Account{ID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var received map[string]any
	if err := json.NewDecoder(result.Body).Decode(&received); err != nil {
		t.Fatal(err)
	}
	details, _ := received["error"].(map[string]any)
	output, _ := received["output"].([]any)
	if result.StatusCode != http.StatusOK || received["status"] != "failed" ||
		details["code"] != "gemini_empty_response" || len(output) != 0 {
		t.Fatalf("tagged Prodex requires an empty-content Gemini failure, got %+v", received)
	}
}
