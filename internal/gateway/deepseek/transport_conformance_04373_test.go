package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04373DeepSeekLoopbackToolRequestAndMalformedGuard(t *testing.T) {
	const secret = "fixture-deepseek-secret"
	var requests int
	var upstreamBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if got := request.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("upstream authorization = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
			return
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Errorf("upstream body = %s: %v", body, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"chatcmpl_fixture","model":"deepseek-chat","choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, secret, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	request := proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Header: http.Header{"Authorization": {"Bearer caller-secret"}},
		Body:   []byte(`{"input":[{"type":"mcp_call","id":"call_sqz_1","name":"mcp__prodex_sqz__compress","arguments":{"text":"large repeated content"},"output":"ref:abc123"}]}`),
	}
	response, err := transport.Execute(context.Background(), request, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || requests != 1 {
		t.Fatalf("status/requests = %d/%d", response.StatusCode, requests)
	}
	if got := upstreamBody["messages"].([]any); len(got) != 2 {
		t.Fatalf("translated messages = %#v", got)
	}

	_, err = transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"input":"hello"} {}`),
	}, proxymodel.Account{})
	var proxyErr *proxymodel.Error
	if !errors.As(err, &proxyErr) || proxyErr.StatusCode != http.StatusBadGateway || proxyErr.Message != "upstream request failed" {
		t.Fatalf("malformed request error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("malformed request reached upstream: %d requests", requests)
	}
}
