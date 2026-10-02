package claude

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

func TestAnthropicModelFallbackChainsMatchProdex(t *testing.T) {
	fixtures := map[string][]string{
		"":        {"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-4-5"},
		"DEFAULT": {"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-4-5"},
		"best":    {"claude-opus-5-5", "claude-sonnet-5-5"},
		"pro":     {"claude-sonnet-5-5", "claude-opus-5-5"},
		"flash":   {"claude-haiku-4-5", "claude-sonnet-5-5"},
	}
	for model, want := range fixtures {
		body, _ := json.Marshal(map[string]any{"model": model, "input": "hello"})
		got := anthropicModelFallbackChain(body)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("model %q chain = %v, want %v", model, got, want)
		}
	}
	combo := anthropicModelFallbackChain([]byte("{\"model\":\"combo:claude-a, claude-b,claude-a\",\"input\":\"x\"}"))
	if strings.Join(combo, ",") != "claude-a,claude-b" {
		t.Fatalf("combo chain = %v", combo)
	}
}

func TestAnthropicResponsesFallsBackAcrossModelsBeforeCredentialRotation(t *testing.T) {
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		model, _ := value["model"].(string)
		models = append(models, model)
		writer.Header().Set("Content-Type", "application/json")
		if len(models) == 1 {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte("{\"error\":{\"code\":\"model_not_supported\"}}"))
			return
		}
		_, _ = writer.Write([]byte("{\"id\":\"chat_ok\",\"model\":\"claude-opus-4-8\",\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}"))
	}))
	defer server.Close()

	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   anthropicMountPath + "/responses",
		Body:   []byte("{\"model\":\"sonnet\",\"input\":\"hello\"}"),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || strings.Join(models, ",") != "claude-sonnet-5-5,claude-opus-5-5" {
		t.Fatalf("status/models = %d / %v", response.StatusCode, models)
	}
}

func TestAnthropicResponsesBare429AndAuthDoNotAdvanceModel(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
	}{
		{"bare429", http.StatusTooManyRequests, "{\"error\":{\"message\":\"too many requests\"}}"},
		{"auth", http.StatusUnauthorized, "{\"error\":{\"type\":\"authentication_error\"}}"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(fixture.status)
				_, _ = writer.Write([]byte(fixture.body))
			}))
			defer server.Close()
			transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
			response, err := transport.Execute(context.Background(), proxymodel.Request{
				Method: http.MethodPost,
				Path:   anthropicMountPath + "/responses",
				Body:   []byte("{\"model\":\"sonnet\",\"input\":\"hello\"}"),
			}, proxymodel.Account{})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if calls != 1 || response.StatusCode != fixture.status {
				t.Fatalf("calls/status = %d / %d", calls, response.StatusCode)
			}
		})
	}
}
