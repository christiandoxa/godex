package proxy

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

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
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
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(writer, `{"error":{"code":"rate_limit_exceeded"}}`)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"object":"response","id":"response-%s"}`, strings.TrimPrefix(token, "Bearer token-"))
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
		_, _ = io.WriteString(writer, `{"object":"response","id":"response-a"}`)
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

func TestProdex04356ProxyBoundAffinityUnauthorizedSignalsFullContextReplay(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		accountID := request.Header.Get("ChatGPT-Account-Id")
		seen = append(seen, accountID)
		writer.Header().Set("Content-Type", "application/json")
		if accountID == "workspace-B" {
			_, _ = io.WriteString(writer, `{"object":"response","id":"response-b"}`)
			return
		}
		if len(seen) == 1 {
			_, _ = io.WriteString(writer, `{"object":"response","id":"response-a"}`)
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"code":"authentication_error"}}`)
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	first := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", "{}", nil)
	_ = first.Body.Close()

	signal := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses",
		`{"previous_response_id":"response-a","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`, nil)
	body, err := io.ReadAll(signal.Body)
	_ = signal.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if signal.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "previous_response_not_found") ||
		!strings.Contains(string(body), "Previous response was not found. Retrying the full request.") {
		t.Fatalf("full-context signal = status %d body %q", signal.StatusCode, body)
	}

	replay := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses",
		`{"input":[{"role":"user","content":"original"},{"role":"assistant","content":"answer"},{"role":"user","content":"continue"}]}`, nil)
	replayBody, err := io.ReadAll(replay.Body)
	_ = replay.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if replay.StatusCode != http.StatusOK || !strings.Contains(string(replayBody), "response-b") {
		t.Fatalf("full-context replay = status %d body %q", replay.StatusCode, replayBody)
	}
	if strings.Join(seen, ",") != "workspace-A,workspace-A,workspace-B" {
		t.Fatalf("auth recovery owner trace = %#v", seen)
	}
}

func TestProxyAffinityKeysEachStayWithTheirOwner(t *testing.T) {
	cases := []proxyAffinityCase{
		{name: "previous response", secondBody: `{"previous_response_id":"response-a"}`, firstReply: `{"object":"response","id":"response-a"}`},
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
	if err == nil {
		t.Fatal("truncated committed stream was reported as successful")
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

func TestProdex04356ProxyRetriesUnauthorizedAccountAfterOAuthRefresh(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	authPath := filepath.Join(accounts[0].Home, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-a","refresh_token":"refresh-a","account_id":"workspace-A"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	refreshCalls := 0
	refreshServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		refreshCalls++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"token-a-refreshed","refresh_token":"refresh-a-next"}`)
	}))
	defer refreshServer.Close()
	t.Setenv("CODEX_REFRESH_TOKEN_URL_OVERRIDE", refreshServer.URL)

	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization := request.Header.Get("Authorization")
		seen = append(seen, authorization)
		if authorization == "Bearer token-a-refreshed" {
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, "ok")
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, "unauthorized")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/responses", "{}", nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "ok" ||
		refreshCalls != 1 ||
		strings.Join(seen, ",") != "Bearer token-a,Bearer token-a-refreshed" {
		t.Fatalf("oauth recovery = status %d body %q refresh=%d seen=%v",
			response.StatusCode, body, refreshCalls, seen)
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
	if calls != len(accounts) {
		t.Fatalf("upstream attempts = %d, want %d", calls, len(accounts))
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
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(writer, `{"error":{"code":"rate_limit_exceeded"}}`)
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

func TestProxyStopsOnPermanentBadRequest(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
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
		t.Fatalf("upstream attempts = %d, want one for a permanent request error", calls)
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

func TestProxyDropsUpstreamContentLengthButKeepsMeaningfulHeaders(t *testing.T) {
	proxy := &Proxy{maxInspect: 1024}
	writer := httptest.NewRecorder()
	lifecycle := &requestLifecycle{}
	response := &proxymodel.Response{
		StatusCode: http.StatusCreated,
		Header: http.Header{
			"Content-Length":      []string{"999"},
			"Content-Encoding":    []string{"identity"},
			"X-Codex-Turn-State":  []string{"synthetic-turn"},
			"X-Upstream-Metadata": []string{"keep-me"},
		},
		Body: io.NopCloser(strings.NewReader("payload")),
		Trailer: http.Header{
			"Content-Length": []string{"trailer-framing-leak"},
			"X-Upstream":     []string{"trailer-value"},
		},
	}

	proxy.forwardResponse(context.Background(), writer, response, nil, "", lifecycle)

	if writer.Code != http.StatusCreated || writer.Body.String() != "payload" {
		t.Fatalf("forwarded response = status %d body %q", writer.Code, writer.Body.String())
	}
	if writer.Header().Get("Content-Length") != "" {
		t.Fatalf("upstream framing header leaked: %q", writer.Header().Get("Content-Length"))
	}
	if writer.Header().Get("Content-Encoding") != "identity" ||
		writer.Header().Get("X-Codex-Turn-State") != "synthetic-turn" ||
		writer.Header().Get("X-Upstream-Metadata") != "keep-me" {
		t.Fatalf("meaningful response headers changed: %#v", writer.Header())
	}
	if writer.Header().Get("X-Upstream") != "trailer-value" {
		t.Fatalf("trailer forwarding changed: %#v", writer.Header())
	}
}

func TestProxyForwardsRequestHeadersWithoutInventingMetadata(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	seen := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen <- request.Header.Clone()
		writer.Header().Set("X-Codex-Turn-State", "turn-value")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	prime := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	prime.Body.Close()
	<-seen

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
	if forwarded.Get("ChatGPT-Account-Id") != "workspace-A" {
		t.Fatal("selected account routing identity missing")
	}
	if forwarded.Get("Authorization") != "Bearer token-a" {
		t.Fatalf("managed authorization was not selected: %q", forwarded.Get("Authorization"))
	}
	if forwarded.Get("X-Codex-Turn-State") != "turn-value" || forwarded.Get("X-Codex-Turn-Metadata") != "metadata-value" ||
		!reflect.DeepEqual(forwarded.Values("X-Codex-Metadata"), []string{"first", "second"}) {
		t.Fatalf("Codex headers were changed: %#v", forwarded)
	}
	for _, key := range []string{"X-Local-Hop", "X-Prodex-Internal-Request-Origin", "Connection"} {
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
	managedProxy, err := newProxyForTest(ProxyConfig{
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
		if _, err := newProxyForTest(ProxyConfig{ListenAddr: address, Accounts: func(context.Context) ([]RuntimeAccount, error) {
			return nil, nil
		}}); err == nil {
			t.Fatalf("listener %q was accepted", address)
		}
	}
}

func TestProxyRejectsInvalidUpstreamURL(t *testing.T) {
	if _, err := newProxyForTest(ProxyConfig{
		UpstreamURL: "not-an-upstream-url",
		Accounts:    func(context.Context) ([]RuntimeAccount, error) { return nil, nil },
	}); err == nil {
		t.Fatal("invalid upstream URL unexpectedly accepted")
	}
}

func TestProxyStopsWhenRequestBodyReadIsCanceled(t *testing.T) {
	proxy, err := newProxyForTest(ProxyConfig{
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
	proxy, err := newProxyForTest(ProxyConfig{
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

func newTestProxy(t *testing.T, upstream string, accounts []RuntimeAccount) *httptest.Server {
	t.Helper()
	return newTestProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
	})
}

func newTestProxyWithConfig(t *testing.T, config ProxyConfig) *httptest.Server {
	t.Helper()
	proxy, err := newProxyForTest(config)
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
	auth := map[string]any{"auth_mode": "chatgpt", "tokens": map[string]string{"access_token": token, "account_id": "workspace-" + id}}
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

func TestProxyRejectsUnsupportedWebsocketPathBeforeUpstream(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream upgrade") }))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	request, err := http.NewRequest(http.MethodGet, proxy.URL+"/unsupported", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unsupported websocket path status = %d", response.StatusCode)
	}
}

func TestProxyRemembersEveryNonstreamResponseInChain(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("ChatGPT-Account-Id") != "workspace-A" {
			t.Error("chain changed upstream account")
		}
		calls++
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"object":"response","id":"response-%d"}`, calls)
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	for _, body := range []string{`{}`, `{"previous_response_id":"response-1"}`, `{"previous_response_id":"response-2"}`} {
		response := doProxyJSON(t, proxy.URL+"/responses", body, nil)
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("chain status = %d", response.StatusCode)
		}
	}
}
