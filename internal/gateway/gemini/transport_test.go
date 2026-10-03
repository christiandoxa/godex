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

func TestGeminiModelsAreServedLocallyWithCanonicalMetadata(t *testing.T) {
	upstreamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		http.Error(writer, "unexpected upstream request", http.StatusBadGateway)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	list, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: mountPath + "/v1/models",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	var body struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if list.StatusCode != http.StatusOK || body.Object != "list" || len(body.Data) != 22 || list.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("Gemini model list = status:%d header:%q body:%#v", list.StatusCode, list.Header.Get("Content-Type"), body)
	}
	if body.Data[0]["id"] != "auto" || body.Data[0]["display_name"] != "Gemini Auto" {
		t.Fatalf("Gemini model metadata = %#v", body.Data[0])
	}

	for path, wantID := range map[string]string{
		mountPath + "/models/gemini-3.5-flash": "gemini-3.5-flash",
		mountPath + "/models/default":          "auto",
		mountPath + "/models/GEMINI-3.5-FLASH": "gemini-3.5-flash",
	} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: path}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		var model map[string]any
		if err := json.NewDecoder(response.Body).Decode(&model); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || model["id"] != wantID {
			t.Errorf("GET %s = status:%d model:%#v; want %q", path, response.StatusCode, model, wantID)
		}
	}

	missing, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: mountPath + "/models/not-a-gemini-model",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	var notFound map[string]any
	if err := json.NewDecoder(missing.Body).Decode(&notFound); err != nil {
		t.Fatal(err)
	}
	errorBody, ok := notFound["error"].(map[string]any)
	if missing.StatusCode != http.StatusNotFound || !ok || errorBody["code"] != "model_not_found" {
		t.Fatalf("missing Gemini model = status:%d body:%#v", missing.StatusCode, notFound)
	}
	padded, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: mountPath + "/models/ gemini-3.5-flash ",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	padded.Body.Close()
	if padded.StatusCode != http.StatusNotFound {
		t.Fatalf("whitespace-padded Gemini model status = %d, want %d", padded.StatusCode, http.StatusNotFound)
	}
	unicodeFold, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: mountPath + "/models/gemini-3.5-flaſh",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer unicodeFold.Body.Close()
	if unicodeFold.StatusCode != http.StatusNotFound {
		t.Fatalf("non-ASCII model case fold status = %d, want %d", unicodeFold.StatusCode, http.StatusNotFound)
	}
	if upstreamCalls != 0 {
		t.Fatalf("local Gemini model requests reached upstream %d times", upstreamCalls)
	}
}

func TestGeminiNonGetModelsRequestPassesThrough(t *testing.T) {
	captured := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured = true
		if request.Method != http.MethodPost || request.URL.Path != "/v1beta/openai/models" {
			t.Errorf("upstream model request = %s %s", request.Method, request.URL.Path)
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
		Method: http.MethodPost, Path: mountPath + "/models", Body: []byte(`{"probe":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "upstream" || !captured {
		t.Fatalf("non-GET model response = %q, captured=%t, err=%v", body, captured, err)
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
