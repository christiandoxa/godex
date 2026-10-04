package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestProxyCompactRecoversAcrossSelectionSweeps(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	type attempt struct {
		account string
		path    string
		body    string
	}
	var mu sync.Mutex
	var attempts []attempt
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
		}
		mu.Lock()
		attempts = append(attempts, attempt{
			account: request.Header.Get("Authorization"),
			path:    request.URL.Path,
			body:    string(body),
		})
		count := len(attempts)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if count <= 4 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(writer, `{"error":{"type":"server_error","code":"overloaded_error"}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"id":"compact-recovered"}`)
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(
		t,
		proxy.URL+"/backend-api/prodex/v1/responses/compact",
		`{"model":"gpt-5.3-codex","input":[]}`,
		nil,
	)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	got := append([]attempt(nil), attempts...)
	mu.Unlock()
	want := []attempt{
		{account: "Bearer token-a", path: "/backend-api/codex/responses/compact", body: `{"model":"gpt-5.3-codex","input":[]}`},
		{account: "Bearer token-b", path: "/backend-api/codex/responses/compact", body: `{"model":"gpt-5.3-codex","input":[]}`},
		{account: "Bearer token-a", path: "/backend-api/codex/responses/compact", body: `{"model":"gpt-5.3-codex","input":[]}`},
		{account: "Bearer token-b", path: "/backend-api/codex/responses/compact", body: `{"model":"gpt-5.3-codex","input":[]}`},
		{account: "Bearer token-a", path: "/backend-api/codex/responses/compact", body: `{"model":"gpt-5.3-codex","input":[]}`},
	}
	if !reflect.DeepEqual(got, want) || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "compact-recovered") {
		t.Fatalf("Compact response/attempt trace = %d %q %#v; want 200 recovered with %#v", response.StatusCode, body, got, want)
	}
}
