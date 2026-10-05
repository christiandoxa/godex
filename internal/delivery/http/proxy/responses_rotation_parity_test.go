package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProxyResponsesPassesGeneric429WithoutRotation(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, "Too Many Requests")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusTooManyRequests || string(body) != "Too Many Requests" {
		t.Fatalf("generic 429 = %d %q", response.StatusCode, body)
	}
	if !reflect.DeepEqual(seen, []string{"Bearer token-a"}) {
		t.Fatalf("generic 429 dispatches = %#v", seen)
	}
}

func TestProxyResponsesRotateStructuredRateLimit(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if token == "Bearer token-a" {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(writer, `{"error":{"code":"rate_limit_exceeded"}}`)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" {
		t.Fatalf("rate-limit recovery = %d %q", response.StatusCode, body)
	}
	if !reflect.DeepEqual(seen, []string{"Bearer token-a", "Bearer token-b"}) {
		t.Fatalf("rate-limit dispatches = %#v", seen)
	}
}

func TestProxyResponsesRecoverAcrossMultipleTransientSweeps(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if len(seen) <= 4 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Bearer token-a", "Bearer token-b", "Bearer token-a", "Bearer token-b", "Bearer token-a"}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" || !reflect.DeepEqual(seen, want) {
		t.Fatalf("overload recovery = %d %q after dispatches %#v", response.StatusCode, body, seen)
	}
}

func TestProxyResponsesKeepsRecoveringPastSixteenSweeps(t *testing.T) {
	account := testRuntimeAccount(t, "A", "token-a")
	const requestBody = `{"model":"gpt-5.3-codex","input":[]}`
	type attempt struct {
		account string
		path    string
		body    string
	}
	var attempts []attempt
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
		}
		attempts = append(attempts, attempt{
			account: request.Header.Get("Authorization"),
			path:    request.URL.Path,
			body:    string(body),
		})
		if len(attempts) <= 17 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()

	now := time.Unix(100, 0)
	var waits []time.Duration
	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return []RuntimeAccount{account}, nil },
		Now:      func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	response := doProxyJSON(t, proxy.URL+"/responses", requestBody, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" {
		t.Fatalf("recovery after 17 transient failures = %d %q; attempts %d", response.StatusCode, body, len(attempts))
	}
	wantPath := "/responses"
	if len(attempts) != 18 || len(waits) != 17 {
		t.Fatalf("recovery attempt/wait count = %d/%d, want 18/17", len(attempts), len(waits))
	}
	for index, got := range attempts {
		if got.account != "Bearer token-a" || got.path != wantPath || got.body != requestBody {
			t.Fatalf("attempt %d = %#v, want account A, path %q, body %q", index+1, got, wantPath, requestBody)
		}
	}
	var waited time.Duration
	for _, delay := range waits {
		if delay <= 0 || delay > 30*time.Second {
			t.Fatalf("recovery wait = %s, want positive and at most 30s", delay)
		}
		waited += delay
	}
	if waited <= 30*time.Second {
		t.Fatalf("recovery stopped before old timeout epoch: total wait %s", waited)
	}
}

func TestProxyResponsesRetryRateLimitAcrossRecoverySweeps(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	now := time.Unix(1_800_000_000, 0)
	var waits []time.Duration
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if len(seen) < 3 {
			writer.Header().Set("Retry-After", "1")
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(writer, `{"error":{"code":"rate_limit_exceeded","message":"Please try again in 1s."}}`)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()

	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL, PreferredAccount: "A",
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
		Now:      func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" {
		t.Fatalf("rate-limit recovery = %d %q, attempts %v", response.StatusCode, body, seen)
	}
	if !reflect.DeepEqual(seen, []string{"Bearer token-a", "Bearer token-b", "Bearer token-a"}) {
		t.Fatalf("rate-limit attempt trace = %#v", seen)
	}
	if !reflect.DeepEqual(waits, []time.Duration{20 * time.Second}) {
		t.Fatalf("rate-limit recovery wait = %v, want the tagged 20-second default", waits)
	}
}

func TestProxyResponsesRecoveryCrossesOldTimeoutWhileQuotaRemains(t *testing.T) {
	account := testRuntimeAccount(t, "A", "token-a")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		if len(seen) < 10 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()
	now := time.Unix(100, 0)
	var waits []time.Duration
	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return []RuntimeAccount{account}, nil },
		Now:      func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, 10)
	for index := range want {
		want[index] = "Bearer token-a"
	}
	var waited time.Duration
	for _, delay := range waits {
		waited += delay
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" ||
		!reflect.DeepEqual(seen, want) || len(waits) != 9 || waited <= 30*time.Second {
		t.Fatalf("recovery status/body/attempts/waits = %d %q %#v %v (%s total)", response.StatusCode, body, seen, waits, waited)
	}
}

func TestProxyResponsesStopsRecoveryWhenRequestIsCanceled(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	var waits int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waitEntered := make(chan struct{}, 1)
	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
		Wait: func(waitContext context.Context, _ time.Duration) error {
			waits++
			if waits == 3 {
				waitEntered <- struct{}{}
				<-waitContext.Done()
				return waitContext.Err()
			}
			return waitContext.Err()
		},
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, proxy.URL+"/responses", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	type responseResult struct {
		response *http.Response
		err      error
	}
	result := make(chan responseResult, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		result <- responseResult{response: response, err: err}
	}()
	<-waitEntered
	cancel()
	response := <-result
	if response.response != nil {
		_ = response.response.Body.Close()
	}
	err = response.err
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled recovery request error = %v, attempts %v, waits %d", err, seen, waits)
	}
	// Prodex recovery includes retry-backoff deadlines. Once both profiles have
	// failed with overload-class 503s, later sweeps wait for recovery instead of
	// dispatching the same profiles again before their cooldown expires.
	want := []string{"Bearer token-a", "Bearer token-b"}
	if !reflect.DeepEqual(seen, want) || waits != 3 {
		t.Fatalf("canceled recovery trace/waits = %#v/%d", seen, waits)
	}
}

func TestProxyResponsesDoNotDispatchAnAuthoritativelyExhaustedPool(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	for index := range accounts {
		accounts[index].EligibleAfter = now.Add(time.Hour)
	}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()
	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
		Now:      func() time.Time { return now },
	})
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || calls != 0 {
		t.Fatalf("all-zero pool status/dispatches = %d/%d, want 503/0", response.StatusCode, calls)
	}
}

func TestProxyResponsesSkipsAnExhaustedAccountForTheViableOne(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	accounts[0].EligibleAfter = now.Add(time.Hour)
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok")
	}))
	defer upstream.Close()
	proxy := newParityProxyWithConfig(t, ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
		Now:      func() time.Time { return now },
	})
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !reflect.DeepEqual(seen, []string{"Bearer token-b"}) {
		t.Fatalf("exhausted account routing = %d %#v", response.StatusCode, seen)
	}
}

func TestProxyStandardHTTPRecoversAcrossTransientSweeps(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		if len(seen) < 3 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/backend-api/prodex/chat/completions", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" ||
		!reflect.DeepEqual(seen, []string{"Bearer token-a", "Bearer token-b", "Bearer token-a"}) {
		t.Fatalf("standard HTTP recovery = %d %q, attempts %#v", response.StatusCode, body, seen)
	}
}

func TestProxyResponsesRecoversAfterPrecommitTransportFailures(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := request.Header.Get("Authorization")
		seen = append(seen, token)
		if len(seen) < 5 {
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack upstream connection: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "recovered")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Bearer token-a", "Bearer token-b", "Bearer token-a", "Bearer token-b", "Bearer token-a"}
	if response.StatusCode != http.StatusOK || string(body) != "recovered" || !reflect.DeepEqual(seen, want) {
		t.Fatalf("transport recovery = %d %q, attempts %#v", response.StatusCode, body, seen)
	}
}

func TestProxyResponsesCommittedStreamDoesNotEnterRecoverySweep(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err == nil || !strings.Contains(string(body), "partial") {
		t.Fatalf("committed stream = %q, %v", body, err)
	}
	if !reflect.DeepEqual(seen, []string{"Bearer token-a"}) {
		t.Fatalf("committed stream replayed: %#v", seen)
	}
}
