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

func TestGeminiSemanticCompactUsesSelectedModelAndSummary(t *testing.T) {
	var models []string
	var parallelToolCalls *bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		models = append(models, requestModel(body))
		var outgoing struct {
			ParallelToolCalls *bool `json:"parallel_tool_calls"`
		}
		_ = json.Unmarshal(body, &outgoing)
		parallelToolCalls = outgoing.ParallelToolCalls
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"chatcmpl_summary","model":"gemini-3.5-flash","choices":[{"message":{"role":"assistant","content":"Keep the current worktree and next step."}}]}`)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact",
		Body: []byte(`{"model":"gemini-3.8-flash","input":[{"type":"message","role":"user","content":"summarize"}],"tools":[{"type":"function","name":"ignored"}],"stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Godex-Compact-Mode") != "semantic" ||
		response.Header.Get("X-Godex-Compact-Provider") != "gemini" || !strings.Contains(string(body), "Keep the current worktree") {
		t.Fatalf("compact response = status:%d headers:%v body:%s", response.StatusCode, response.Header, body)
	}
	if len(models) != 1 || models[0] != "gemini-3.8-flash" {
		t.Fatalf("compact fallback models = %#v", models)
	}
	if parallelToolCalls == nil || *parallelToolCalls {
		t.Fatalf("parallel_tool_calls = %v, want false", parallelToolCalls)
	}
}

func TestGeminiCompactUsesLocalFallbackOnRequestFailure(t *testing.T) {
	transport, err := NewRuntimeTransport("https://gemini.example.test/v1beta", "fixture-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact", Body: []byte(`{"input":"not-an-array"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Godex-Compact-Mode") != "local-fallback" || response.Header.Get("X-Godex-Compact-Degraded") != "true" ||
		response.Header.Get("X-Godex-Compact-Reason") != "invalid-request" {
		t.Fatalf("compact fallback headers = %v", response.Header)
	}
}
