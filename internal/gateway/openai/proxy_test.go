package openai

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxyRoundRobinAndPreCommitRotation(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var mu sync.Mutex
	seen := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		mu.Lock()
		seen = append(seen, token)
		mu.Unlock()
		if token == "Bearer token-a" && len(seen) == 3 {
			writer.Header().Set("Retry-After", "60")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"id":"response-%s"}`, strings.TrimPrefix(token, "Bearer token-"))
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	client := &http.Client{}
	post := func() *http.Response {
		response, err := client.Post(proxy.URL+"/backend-api/prodex/responses", "application/json", strings.NewReader(`{"model":"test"}`))
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := post()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d", first.StatusCode)
	}
	_ = first.Body.Close()
	second := post()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("second status = %d", second.StatusCode)
	}
	_ = second.Body.Close()
	third := post()
	if third.StatusCode != http.StatusOK {
		t.Fatalf("rotated status = %d", third.StatusCode)
	}
	_ = third.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 || seen[0] != "Bearer token-a" || seen[1] != "Bearer token-b" || seen[2] != "Bearer token-a" || seen[3] != "Bearer token-b" {
		t.Fatalf("upstream authorization sequence = %#v", seen)
	}
}

func TestProxyContinuationAffinity(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.Header().Set("x-codex-turn-state", "turn-a")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"response-a"}`)
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	first := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{"session_id":"session-a"}`, nil)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d", first.StatusCode)
	}
	_ = first.Body.Close()
	second := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{"previous_response_id":"response-a","session_id":"session-a"}`, map[string]string{"x-codex-turn-state": "turn-a"})
	if second.StatusCode != http.StatusOK {
		t.Fatalf("continuation status = %d", second.StatusCode)
	}
	_ = second.Body.Close()
	if len(seen) != 2 || seen[0] != "Bearer token-a" || seen[1] != "Bearer token-a" {
		t.Fatalf("affinity authorization sequence = %#v", seen)
	}
}

func TestProxyBoundAffinityPreservesUnauthorizedResponse(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"id":"response-a"}`)
			return
		}
		writer.Header().Set("X-Upstream", "bound-unauthorized")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, "bound unauthorized")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	first := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	_ = first.Body.Close()
	second := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{"previous_response_id":"response-a"}`, nil)
	body, err := io.ReadAll(second.Body)
	_ = second.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if second.StatusCode != http.StatusUnauthorized || string(body) != "bound unauthorized" || second.Header.Get("X-Upstream") != "bound-unauthorized" {
		t.Fatalf("bound response = status %d, body %q, header %q", second.StatusCode, body, second.Header.Get("X-Upstream"))
	}
}

func TestProxyAffinityKeysEachStayWithTheirOwner(t *testing.T) {
	cases := []proxyAffinityCase{
		{name: "previous response", secondBody: `{"previous_response_id":"response-a"}`, firstReply: `{"id":"response-a"}`},
		{name: "turn state", firstHeaders: map[string]string{"x-codex-turn-state": ""}, firstResponseHeaders: map[string]string{"x-codex-turn-state": "turn-a"}, secondHeaders: map[string]string{"x-codex-turn-state": "turn-a"}},
		{name: "session", firstBody: `{"session_id":"session-a"}`, secondBody: `{"session_id":"session-a"}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			runProxyAffinityCase(t, testCase)
		})
	}
}

type proxyAffinityCase struct {
	name                 string
	firstBody            string
	firstHeaders         map[string]string
	secondBody           string
	secondHeaders        map[string]string
	firstReply           string
	firstResponseHeaders map[string]string
}

func runProxyAffinityCase(t *testing.T, testCase proxyAffinityCase) {
	t.Helper()
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(proxyAffinityHandler(testCase, &seen))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	first := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", testCase.firstBody, testCase.firstHeaders)
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()
	second := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", testCase.secondBody, testCase.secondHeaders)
	_ = second.Body.Close()
	if first.StatusCode != http.StatusOK || second.StatusCode != http.StatusOK {
		t.Fatalf("statuses = %d, %d", first.StatusCode, second.StatusCode)
	}
	if len(seen) != 2 || seen[0] != "Bearer token-a" || seen[1] != "Bearer token-a" {
		t.Fatalf("affinity authorization sequence = %#v", seen)
	}
}

func proxyAffinityHandler(testCase proxyAffinityCase, seen *[]string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		*seen = append(*seen, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		if len(*seen) == 1 {
			for key, value := range testCase.firstResponseHeaders {
				writer.Header().Set(key, value)
			}
			_, _ = io.WriteString(writer, testCase.firstReply)
			return
		}
		_, _ = io.WriteString(writer, `{}`)
	})
}

func TestProxyDoesNotReplayCommittedStream(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("upstream lacks flusher")
		}
		_, _ = io.WriteString(writer, "data: {\"id\":\"stream-a\"}\n\n")
		flusher.Flush()
		panic(http.ErrAbortHandler)
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !strings.Contains(string(body), "stream-a") {
		t.Fatalf("stream body = %q", body)
	}
	if len(seen) != 1 || seen[0] != "Bearer token-a" {
		t.Fatalf("stream was replayed: %#v", seen)
	}
}

func TestProxyPassesNonRetryableClientErrorOnce(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"code":"invalid_request"}}`)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if calls != 1 {
		t.Fatalf("upstream calls = %d", calls)
	}
}

func TestProxyPreservesLastStructuredQuotaAfterRotation(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"code":"insufficient_quota"}}`)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || string(body) != `{"error":{"code":"insufficient_quota"}}` {
		t.Fatalf("quota response = status %d, body %q", response.StatusCode, body)
	}
	if calls != len(accounts) {
		t.Fatalf("quota rotation attempts = %d, want %d", calls, len(accounts))
	}
}

func TestProxyRetriesUnauthorizedAccountAfterReload(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if len(seen) == 1 {
			if err := os.WriteFile(filepath.Join(accounts[0].Home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-a-reloaded"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if len(seen) != 2 || seen[0] != "Bearer token-a" || seen[1] != "Bearer token-a-reloaded" {
		t.Fatalf("reload sequence = %#v", seen)
	}
}

func TestProxyReturnsLastUnauthorizedAfterRotation(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.Header().Set("X-Upstream", "unauthorized")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, "unauthorized")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || string(body) != "unauthorized" || response.Header.Get("X-Upstream") != "unauthorized" {
		t.Fatalf("response = status %d, body %q, header %q", response.StatusCode, body, response.Header.Get("X-Upstream"))
	}
	if calls != len(accounts)*2 {
		t.Fatalf("upstream attempts = %d, want %d", calls, len(accounts)*2)
	}
}

func TestProxyRetryAfterQuarantinesAccount(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if token == "Bearer token-a" {
			writer.Header().Set("Retry-After", "60")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	for range 2 {
		response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", response.StatusCode)
		}
		_ = response.Body.Close()
	}
	if len(seen) != 3 || seen[0] != "Bearer token-a" || seen[1] != "Bearer token-b" || seen[2] != "Bearer token-b" {
		t.Fatalf("quarantine authorization sequence = %#v", seen)
	}
}

func TestProxyBoundsRetryAttempts(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if calls != len(accounts) {
		t.Fatalf("upstream attempts = %d, want %d", calls, len(accounts))
	}
}

func TestProxyForwardsHeadersBodyAndTrailers(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Upstream", "present")
		writer.Header().Set("X-Codex-Turn-State", "turn-value")
		writer.Header().Add("X-Codex-Metadata", "first")
		writer.Header().Add("X-Codex-Metadata", "second")
		writer.Header().Set("Content-Encoding", "identity")
		writer.Header().Set("Server", "upstream-server-value")
		writer.Header().Set("Date", "upstream-date-value")
		writer.Header().Set("Trailer", "X-Upstream-Trailer")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, "payload")
		writer.Header().Set("X-Upstream-Trailer", "trailer-value")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated || string(body) != "payload" || response.Header.Get("X-Upstream") != "present" {
		t.Fatalf("response = status %d, body %q, header %q", response.StatusCode, body, response.Header.Get("X-Upstream"))
	}
	if response.Header.Get("X-Codex-Turn-State") != "turn-value" ||
		!reflect.DeepEqual(response.Header.Values("X-Codex-Metadata"), []string{"first", "second"}) ||
		response.Header.Get("Content-Encoding") != "identity" {
		t.Fatalf("end-to-end response headers were changed: %#v", response.Header)
	}
	if response.Header.Get("Server") != "upstream-server-value" || response.Header.Get("Date") != "upstream-date-value" {
		t.Fatalf("end-to-end transport headers were changed: %#v", response.Header)
	}
	if response.Trailer.Get("X-Upstream-Trailer") != "trailer-value" {
		t.Fatalf("trailer = %q", response.Trailer.Get("X-Upstream-Trailer"))
	}
}

func TestProxyForwardsRequestHeadersWithoutInventingMetadata(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	seen := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen <- request.Header.Clone()
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)

	request, err := http.NewRequest(http.MethodPost, proxy.URL+"/backend-api/prodex/responses", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Codex-Turn-State", "turn-value")
	request.Header.Set("X-Codex-Turn-Metadata", "metadata-value")
	request.Header.Add("X-Codex-Metadata", "first")
	request.Header.Add("X-Codex-Metadata", "second")
	request.Header.Set("Connection", "keep-alive, X-Local-Hop")
	request.Header.Set("X-Local-Hop", "must-not-forward")
	request.Header.Set("ChatGPT-Account-Id", "caller-value")
	request.Header.Set("X-Prodex-Internal-Request-Origin", "caller-value")
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	forwarded := <-seen
	if forwarded.Get("Authorization") != "Bearer token-a" {
		t.Fatalf("managed authorization was not selected: %q", forwarded.Get("Authorization"))
	}
	if forwarded.Get("X-Codex-Turn-State") != "turn-value" || forwarded.Get("X-Codex-Turn-Metadata") != "metadata-value" ||
		!reflect.DeepEqual(forwarded.Values("X-Codex-Metadata"), []string{"first", "second"}) {
		t.Fatalf("Codex headers were changed: %#v", forwarded)
	}
	for _, key := range []string{"X-Local-Hop", "ChatGPT-Account-Id", "X-Prodex-Internal-Request-Origin", "Connection"} {
		if forwarded.Get(key) != "" {
			t.Fatalf("local header %s was forwarded: %q", key, forwarded.Get(key))
		}
	}
	if forwarded.Get("Accept-Encoding") != "" {
		t.Fatalf("proxy transport invented Accept-Encoding: %q", forwarded.Get("Accept-Encoding"))
	}
}

func TestProxyPreservesCompressedResponse(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("compressed payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Encoding", "gzip")
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write(compressed.Bytes())
	}))
	defer upstream.Close()
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	proxy := newTestProxy(t, upstream.URL, accounts)
	request, err := http.NewRequest(http.MethodPost, proxy.URL+"/backend-api/prodex/responses", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Content-Encoding") != "gzip" || !bytes.Equal(body, compressed.Bytes()) {
		t.Fatalf("compressed response changed: encoding %q, body length %d", response.Header.Get("Content-Encoding"), len(body))
	}
}

func TestUpstreamPathMatchesCodexMount(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{name: "responses", base: "/backend-api", path: "/backend-api/prodex/responses", want: "/backend-api/codex/responses"},
		{name: "legacy version", base: "/backend-api", path: "/backend-api/prodex/v1/responses", want: "/backend-api/codex/responses"},
		{name: "already normalized", base: "/backend-api", path: "/backend-api/codex/responses", want: "/backend-api/codex/responses"},
		{name: "custom base", base: "/backend-api-v2", path: "/backend-api/prodex/responses", want: "/backend-api-v2/backend-api/codex/responses"},
		{name: "standard v1", base: "/backend-api", path: "/v1/responses", want: "/backend-api/v1/responses"},
		{name: "pathless base", base: "", path: "/v1/responses", want: "/v1/responses"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := upstreamPath(testCase.base, testCase.path); got != testCase.want {
				t.Fatalf("upstreamPath(%q, %q) = %q, want %q", testCase.base, testCase.path, got, testCase.want)
			}
		})
	}
}

func TestProxyForwardsStreamTrailers(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Trailer", "X-Stream-Trailer")
		flusher := writer.(http.Flusher)
		_, _ = io.WriteString(writer, "data: {}\n\n")
		flusher.Flush()
		writer.Header().Set("X-Stream-Trailer", "stream-value")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", `{}`, nil)
	_, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Trailer.Get("X-Stream-Trailer") != "stream-value" {
		t.Fatalf("stream trailer = %q", response.Trailer.Get("X-Stream-Trailer"))
	}
}

func TestProxyQuarantineIsBounded(t *testing.T) {
	proxy := &Proxy{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]time.Time)}
	for index := 0; index <= maxQuarantinedAccounts; index++ {
		proxy.quarantineAccount(fmt.Sprintf("account-%d", index), time.Minute)
	}
	proxy.mu.Lock()
	count := len(proxy.quarantine)
	proxy.mu.Unlock()
	if count != maxQuarantinedAccounts {
		t.Fatalf("quarantine entries = %d, want %d", count, maxQuarantinedAccounts)
	}
}

func TestProxyQuarantineKeepsLongerExistingLease(t *testing.T) {
	proxy := &Proxy{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]time.Time)}
	proxy.quarantineAccount("synthetic", time.Hour)
	proxy.quarantineAccount("synthetic", time.Second)
	if !proxy.isQuarantined("synthetic", time.Unix(10, 0).Add(time.Minute)) {
		t.Fatal("longer quarantine lease was shortened")
	}
}

func TestSortRuntimeAccountsDeterministicallyDeduplicates(t *testing.T) {
	accounts := sortRuntimeAccounts([]RuntimeAccount{
		{ID: "same", Home: "/z", Enabled: false},
		{ID: "same", Home: "/a", Enabled: true},
	})
	if len(accounts) != 1 || accounts[0].Home != "/a" || !accounts[0].Enabled {
		t.Fatalf("sorted accounts = %#v", accounts)
	}
}

func TestSortRuntimeAccountsUsesHomeAsTieBreaker(t *testing.T) {
	accounts := sortRuntimeAccounts([]RuntimeAccount{
		{ID: "same", Home: "/z", Enabled: true},
		{ID: "same", Home: "/a", Enabled: true},
	})
	if len(accounts) != 1 || accounts[0].Home != "/a" {
		t.Fatalf("sorted accounts = %#v", accounts)
	}
}

func TestProxyCancellationStopsUpstream(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	started := make(chan struct{})
	canceled := make(chan struct{})
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		close(canceled)
		return nil, request.Context().Err()
	})
	managedProxy, err := NewProxy(ProxyConfig{
		ListenAddr:  "127.0.0.1:0",
		UpstreamURL: "http://upstream.test/backend-api",
		Client:      &http.Client{Transport: transport},
		Accounts: func(context.Context) ([]RuntimeAccount, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(managedProxy)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/backend-api/prodex/responses", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan struct{}, 1)
	go func() {
		managedProxy.ServeHTTP(httptest.NewRecorder(), request)
		result <- struct{}{}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request was not canceled")
	}
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("downstream request did not finish")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProxyRejectsNonLoopbackListener(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", ":0", "localhost:0"} {
		if _, err := NewProxy(ProxyConfig{ListenAddr: address, Accounts: func(context.Context) ([]RuntimeAccount, error) {
			return nil, nil
		}}); err == nil {
			t.Fatalf("listener %q was accepted", address)
		}
	}
}

func TestProxyRejectsInvalidUpstreamURL(t *testing.T) {
	if _, err := NewProxy(ProxyConfig{
		UpstreamURL: "not-an-upstream-url",
		Accounts:    func(context.Context) ([]RuntimeAccount, error) { return nil, nil },
	}); err == nil {
		t.Fatal("invalid upstream URL unexpectedly accepted")
	}
}

func TestProxyStopsWhenRequestBodyReadIsCanceled(t *testing.T) {
	proxy, err := NewProxy(ProxyConfig{
		UpstreamURL: "http://upstream.test/backend-api",
		Accounts:    func(context.Context) ([]RuntimeAccount, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1/backend-api/prodex/responses", errorReadCloser{})
	if err != nil {
		t.Fatal(err)
	}
	proxy.ServeHTTP(httptest.NewRecorder(), request)
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) { return 0, errors.New("synthetic body read failure") }
func (errorReadCloser) Close() error             { return nil }

func TestProxyStartsOnLoopbackAndCloses(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy, err := NewProxy(ProxyConfig{
		ListenAddr:  "127.0.0.1:0",
		UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	response := doProxyJSON(t, proxy.Endpoint()+"/backend-api/prodex/responses", `{}`, nil)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent || !strings.HasPrefix(proxy.Endpoint(), "http://127.0.0.1:") {
		t.Fatalf("proxy endpoint = %q, status = %d", proxy.Endpoint(), response.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := proxy.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyPreCommitFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		kind       responseKind
		quarantine bool
	}{
		{name: "server error", status: http.StatusBadGateway, kind: responseRetry},
		{name: "rate limit", status: http.StatusTooManyRequests, kind: responseRetry, quarantine: true},
		{name: "unauthorized", status: http.StatusUnauthorized, kind: responseAuthFailure},
		{name: "client error", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_request"}}`, kind: responsePass},
		{name: "quota body", status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`, kind: responseRetry, quarantine: true},
	}
	proxy := &Proxy{now: func() time.Time { return time.Unix(10, 0) }, maxInspect: 1024}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: testCase.status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(testCase.body)),
			}
			if testCase.name == "rate limit" {
				response.Header.Set("Retry-After", "5")
			}
			outcome, pending := proxy.classify(response)
			if outcome.kind != testCase.kind || (outcome.quarantine > 0) != testCase.quarantine {
				t.Fatalf("outcome = %#v, pending = %#v", outcome, pending)
			}
			if pending != nil {
				pending.close()
			}
		})
	}
}

func newTestProxy(t *testing.T, upstream string, accounts []RuntimeAccount) *httptest.Server {
	t.Helper()
	proxy, err := NewProxy(ProxyConfig{
		ListenAddr:  "127.0.0.1:0",
		UpstreamURL: upstream,
		Accounts: func(context.Context) ([]RuntimeAccount, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	return server
}

func testRuntimeAccounts(t *testing.T, firstID, firstToken, secondID, secondToken string) []RuntimeAccount {
	t.Helper()
	return []RuntimeAccount{
		testRuntimeAccount(t, firstID, firstToken),
		testRuntimeAccount(t, secondID, secondToken),
	}
}

func testRuntimeAccount(t *testing.T, id, token string) RuntimeAccount {
	t.Helper()
	home := t.TempDir()
	auth := map[string]any{"auth_mode": "chatgpt", "tokens": map[string]string{"access_token": token}}
	content, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return RuntimeAccount{ID: id, Home: home, Enabled: true}
}

func doProxyJSON(t *testing.T, endpoint, body string, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
