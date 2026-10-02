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

func TestGeminiTargetNormalizesOpenAIPathAndEscapesModelID(t *testing.T) {
	for _, test := range []struct {
		base string
		want string
	}{
		{"https://gemini.example.test/v1beta/", "https://gemini.example.test/v1beta/openai/chat/completions?key=value"},
		{"https://gemini.example.test/v1beta/openai/", "https://gemini.example.test/v1beta/openai/chat/completions?key=value"},
	} {
		transport, err := NewRuntimeTransport(test.base, "fixture-key", nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := transport.target(route{kind: routeResponses}, "key=value")
		transport.Close()
		if err != nil || got != test.want {
			t.Fatalf("target for %q = %q, err=%v; want %q", test.base, got, err, test.want)
		}
	}

	transport, err := NewRuntimeTransport("https://gemini.example.test/v1beta", "fixture-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	got, err := transport.target(route{kind: routeModel, modelID: "gemini model"}, "")
	if err != nil || got != "https://gemini.example.test/v1beta/openai/models/gemini%20model" {
		t.Fatalf("model target = %q, err=%v", got, err)
	}
}

func TestGeminiRuntimeURLRejectsUnsafeOrUnsupportedBase(t *testing.T) {
	for _, base := range []string{
		"ftp://gemini.example.test/v1beta",
		"https://user:pass@gemini.example.test/v1beta",
		"https://gemini.example.test/v1beta?key=secret",
		"https://gemini.example.test/v1beta#fragment",
	} {
		if _, err := NewRuntimeTransport(base, "fixture-key", nil); err == nil {
			t.Errorf("NewRuntimeTransport(%q) succeeded, want URL validation error", base)
		}
	}
}

func TestGeminiRouteRejectsModelPathSegments(t *testing.T) {
	for _, id := range []string{"", ".", "..", "nested/model"} {
		if _, err := runtimeRoute(mountPath + "/models/" + id); err == nil {
			t.Errorf("runtimeRoute model ID %q succeeded, want rejection", id)
		}
	}
}

func TestGeminiResponsesUsesRequestedModelAndBearerAuth(t *testing.T) {
	type captured struct {
		path  string
		auth  string
		model string
	}
	var requests []captured
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &value)
		requests = append(requests, captured{
			path: request.URL.Path, auth: request.Header.Get("Authorization"), model: value.Model,
		})
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"gemini-3.7-flash","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL+"/v1beta/openai", "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"model":"gemini-3.8-flash","input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"object":"response"`) {
		t.Fatalf("translated response = status:%d body:%s", response.StatusCode, body)
	}
	if len(requests) != 1 || requests[0].model != "gemini-3.8-flash" {
		t.Fatalf("upstream requests = %#v", requests)
	}
	for _, request := range requests {
		if request.path != "/v1beta/openai/chat/completions" || request.auth != "Bearer fixture-key" {
			t.Fatalf("upstream request = %#v", request)
		}
	}
}

func TestGeminiUnstructured429IsReturnedWithoutFallback(t *testing.T) {
	const body = "Quota exhausted for this account\n"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "text/plain")
		writer.Header().Set("X-Upstream-Error", "quota")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, body)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"model":"auto","input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusTooManyRequests || string(got) != body ||
		response.Header.Get("Content-Type") != "text/plain" || response.Header.Get("X-Upstream-Error") != "quota" || calls != 1 {
		t.Fatalf("unstructured 429 = status:%d headers:%v body:%q calls:%d", response.StatusCode, response.Header, got, calls)
	}
}

func TestGeminiMessagesPassthroughUsesNativeAPIKeyHeader(t *testing.T) {
	captured := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured = true
		if request.URL.Path != "/v1beta/v1/messages" || request.Header.Get("x-goog-api-key") != "fixture-key" || request.Header.Get("Authorization") != "" {
			t.Errorf("native upstream request path=%q headers=%v", request.URL.Path, request.Header)
		}
		_, _ = io.WriteString(writer, "upstream")
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL+"/v1beta", "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/v1/messages", Body: []byte(`{"model":"gemini-3.5-flash"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "upstream" || !captured {
		t.Fatalf("passthrough response = %q, captured=%t, err=%v", body, captured, err)
	}
}
