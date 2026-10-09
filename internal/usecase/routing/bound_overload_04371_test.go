package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const headerlessCapacityEvent04371 = "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\"}}}\n\n"
const successOutputEvent04371 = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered-same-turn\"}\n\n"

type boundOverload04371Gateway struct {
	mu           sync.Mutex
	owners       []string
	alwaysFail   bool
	firstPayload string
	retryAfter   string
	contentType  string
}

func (g *boundOverload04371Gateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	g.mu.Lock()
	g.owners = append(g.owners, account.ID)
	attempt := len(g.owners)
	g.mu.Unlock()
	payload := successOutputEvent04371
	if attempt == 1 || g.alwaysFail {
		payload = headerlessCapacityEvent04371
		if g.firstPayload != "" {
			payload = g.firstPayload
		}
	}
	header := make(http.Header)
	if g.retryAfter != "" {
		header.Set("Retry-After", g.retryAfter)
	}
	if g.contentType != "" {
		header.Set("Content-Type", g.contentType)
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: header,
		Body: io.NopCloser(strings.NewReader(payload)),
	}, nil
}
func TestProdex04371TurnStateBoundHeaderlessOverloadRetriesSameOwner(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const alternate = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	gateway := &boundOverload04371Gateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: owner, Home: "/owner", Enabled: true},
				{ID: alternate, Home: "/alternate", Enabled: true},
			}, nil
		},
		PreferredAccount:         owner,
		ProfileInflightHardLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sticky-turn"}, router.now()); err != nil {
		t.Fatal(err)
	}
	req := proxymodel.Request{
		RequestID: 0, Path: "/backend-api/codex/responses",
		Header:         http.Header{"X-Codex-Turn-State": []string{"sticky-turn"}},
		Body:           []byte(`{"stream":true,"input":[]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	exchange, err := router.Forward(ctx, req)
	if err != nil {
		t.Fatalf("bound same-owner recovery: %v", err)
	}
	defer exchange.Close()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(exchange.Result.Prefix) + string(body)
	if !strings.Contains(output, "recovered-same-turn") || strings.Contains(output, "server_is_overloaded") {
		t.Fatalf("overload leaked instead of same-owner retry: %q", output)
	}
	gateway.mu.Lock()
	owners := append([]string(nil), gateway.owners...)
	gateway.mu.Unlock()
	if strings.Join(owners, ",") != owner+","+owner {
		t.Fatalf("precommit retry changed owner or lost attempt: %v", owners)
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	router.mu.Lock()
	current := router.inflight[owner]
	releases := router.profileInflightReleasesTotal
	admitted := router.profileInflightAdmissionsTotal
	router.mu.Unlock()
	if current != 0 || releases != 2 || admitted != 2 {
		t.Fatalf("same-owner retry leaked admission: current=%d acquired=%d released=%d", current, admitted, releases)
	}
}

func TestProdex04371BoundOverloadRetryPlannerConstraints(t *testing.T) {
	tests := []struct {
		name                      string
		hard, previous, committed bool
		retries                   int
		elapsed                   time.Duration
		advice                    time.Duration
		advicePresent             bool
		requestID                 uint64
		want                      time.Duration
		eligible                  bool
	}{
		{"base0", true, false, false, 0, 0, 0, false, 0, 250 * time.Millisecond, true},
		{"base1", true, false, false, 1, 0, 0, false, 0, 500 * time.Millisecond, true},
		{"base2", true, false, false, 2, 0, 0, false, 0, time.Second, true},
		{"base3", true, false, false, 3, 0, 0, false, 0, 2 * time.Second, true},
		{"base4", true, false, false, 4, 0, 0, false, 0, 4 * time.Second, true},
		{"jitter", true, false, false, 0, 0, 0, false, 125, 375 * time.Millisecond, true},
		{"upstream_advice", true, false, false, 0, 0, 2 * time.Second, true, 0, 2 * time.Second, true},
		{"zero_advice", true, false, false, 0, 0, 0, true, 0, 250 * time.Millisecond, true},
		{"ceil_advice", true, false, false, 0, 0, 250*time.Millisecond + time.Nanosecond, true, 0, 251 * time.Millisecond, true},
		{"too_long_advice", true, false, false, 0, 0, 300 * time.Second, true, 0, 0, false},
		{"overflow_advice", true, false, false, 0, 0, time.Duration(1<<63 - 1), true, 0, 0, false},
		{"deadline_short", true, false, false, 0, 59999 * time.Millisecond, 0, false, 0, 0, false},
		{"deadline_ceil", true, false, false, 0, 59749*time.Millisecond + time.Nanosecond, 0, false, 0, 0, false},
		{"deadline_exceeded", true, false, false, 0, time.Minute, 0, false, 0, 0, false},
		{"too_many", true, false, false, 5, 0, 0, false, 0, 0, false},
		{"previous_response", true, true, false, 0, 0, 0, false, 0, 0, false},
		{"no_affinity", false, false, false, 0, 0, 0, false, 0, 0, false},
		{"visible_output", true, false, true, 0, 0, 0, false, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, ok := boundOverloadRetryDelay(tt.hard, tt.previous, tt.committed, tt.retries, tt.elapsed, tt.advice, tt.advicePresent, tt.requestID)
			if ok != tt.eligible || (ok && delay != tt.want) {
				t.Fatalf("retry plan %s: (%v,%v) want (%v,%v)", tt.name, delay, ok, tt.want, tt.eligible)
			}
		})
	}
}

// A retryable-but-not-overloaded SSE failure must not turn into an
// undocumented same-owner replay; Prodex 0.437.1 restricts this path
// to response.failed/server_is_overloaded.
func TestProdex04371BoundSameTurnDoesNotRetryOtherSSEFailures(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, fixture := range []struct{ name, first string }{
		{"rate_limit", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"rate_limit_exceeded\"}}}\n\n"},
		{"visible_output", successOutputEvent04371 + headerlessCapacityEvent04371},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &boundOverload04371Gateway{firstPayload: fixture.first}
			router, err := NewRouter(Config{
				Gateway: gateway,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: owner, Home: "/synthetic", Enabled: true}}, nil
				},
				PreferredAccount: owner,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sticky"}, router.now()); err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Path:           "/backend-api/codex/responses",
				Header:         http.Header{"X-Codex-Turn-State": []string{"sticky"}},
				Body:           []byte(`{"stream":true,"input":[]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(exchange.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			wire := string(exchange.Result.Prefix) + string(body)
			if !strings.Contains(wire, strings.TrimSpace(fixture.first)) {
				t.Fatalf("terminal precommit event was hidden: %q", wire)
			}
			if err := exchange.Close(); err != nil {
				t.Fatal(err)
			}
			if len(gateway.owners) != 1 {
				t.Fatalf("unrelated SSE failure or committed output was replayed: %v", gateway.owners)
			}
		})
	}
}

func TestProdex04371BoundOverloadLongRetryAfterLeavesUpstreamFailure(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	gateway := &boundOverload04371Gateway{retryAfter: "300"}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: owner, Home: "/synthetic", Enabled: true}}, nil
		}, PreferredAccount: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sticky"}, router.now()); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Path:           "/backend-api/codex/responses",
		Header:         http.Header{"X-Codex-Turn-State": []string{"sticky"}},
		Body:           []byte(`{"stream":true,"input":[]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if len(gateway.owners) != 1 || !strings.Contains(string(exchange.Result.Prefix), "server_is_overloaded") {
		t.Fatalf("a 300s Retry-After was truncated to force an unsafe retry: %v", gateway.owners)
	}
}

func TestProdex04371BoundOverloadStopsAfterFiveSameOwnerRetries(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const alternate = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	gateway := &boundOverload04371Gateway{alwaysFail: true}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: owner, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: owner, Home: "/owner", Enabled: true},
				{ID: alternate, Home: "/alternate", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sticky"}, router.now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	exchange, err := router.Forward(ctx, proxymodel.Request{
		RequestID: 0, Path: "/backend-api/codex/responses",
		Header:         http.Header{"X-Codex-Turn-State": []string{"sticky"}},
		Body:           []byte(`{"stream":true,"input":[]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatalf("retry exhaustion must return upstream failure: %v", err)
	}
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exchange.Result.Prefix)+string(body), "server_is_overloaded") {
		t.Fatal("bounded retries masked the original upstream overload")
	}
	if err := exchange.Close(); err != nil {
		t.Fatal(err)
	}
	gateway.mu.Lock()
	calls := append([]string(nil), gateway.owners...)
	gateway.mu.Unlock()
	if len(calls) != 6 {
		t.Fatalf("retry count %d, want initial+five", len(calls))
	}
	for _, accountID := range calls {
		if accountID != owner {
			t.Fatalf("turn-state retry moved ownership: %v", calls)
		}
	}
	router.mu.Lock()
	active := router.inflight[owner]
	acquired := router.profileInflightAdmissionsTotal
	released := router.profileInflightReleasesTotal
	router.mu.Unlock()
	if active != 0 || acquired != 6 || released != 6 {
		t.Fatalf("retry exhaustion leaked profile admission: active=%d acquired=%d released=%d", active, acquired, released)
	}
}

// In Prodex 0.437.1 the source HTTP Retry-After takes precedence over
// retry advice embedded in a precommit SSE event. Long advice must be
// honored by refusing to attempt outside the 60-second planning window.
func TestProdex04371BoundOverloadHeaderAndEmbeddedAdvicePrecedence(t *testing.T) {
	stream := "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Please retry in 1s.\",\"headers\":{\"Retry-After\":\"5\"}}}}\n\n"
	prefix := &pendingResponse{prefix: []byte(stream)}
	now := time.Unix(1_600_000_000, 0)
	fixtures := []struct {
		name     string
		header   string
		want     time.Duration
		eligible bool
	}{
		{"embedded_advice", "", 5 * time.Second, true},
		{"source_header_overrides", "2", 2 * time.Second, true},
		{"long_source_header", "300", 300 * time.Second, false},
		{"bad_source_header_falls_back", "invalid", 5 * time.Second, true},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{Header: make(http.Header)}
			if fixture.header != "" {
				response.Header.Set("Retry-After", fixture.header)
			}
			delay, present := boundOverloadAdvice(response, prefix, now)
			if !present || delay != fixture.want {
				t.Fatalf("retry metadata: got=%v/%t want=%v/true", delay, present, fixture.want)
			}
			_, eligible := boundOverloadRetryDelay(true, false, false, 0, 0, delay, present, 0)
			if eligible != fixture.eligible {
				t.Fatalf("retry planning accepted oversize advice: eligible=%t want=%t", eligible, fixture.eligible)
			}
		})
	}
}

// Tagged Prodex 0.437.1 considers an explicit upstream SSE MIME authoritative
// even when the client's original request omitted stream:true. Bound
// turn-state retry policy must not depend on a redundant input stream flag
// once the actual upstream is known to be an uncommitted SSE overload.
func TestProdex04371ExplicitUpstreamSSEBoundTurnRetryWithoutStreamTrue(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	gateway := &boundOverload04371Gateway{contentType: "text/event-stream"}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: owner,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: owner, Home: "/synthetic", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sse-explicit"}, router.now()); err != nil {
		t.Fatal(err)
	}
	req := proxymodel.Request{
		Path:           "/backend-api/codex/responses",
		Header:         http.Header{"X-Codex-Turn-State": []string{"sse-explicit"}},
		Body:           []byte(`{"stream":false,"input":[]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	exchange, err := router.Forward(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	combined := string(exchange.Result.Prefix) + string(body)
	if !strings.Contains(combined, "recovered-same-turn") || strings.Contains(combined, "server_is_overloaded") {
		t.Fatalf("explicit upstream SSE wasn't retried on same owner: %q", combined)
	}
	if len(gateway.owners) != 2 || gateway.owners[0] != owner || gateway.owners[1] != owner {
		t.Fatalf("retry crossed owner or never happened: %v", gateway.owners)
	}
}

// A server_is_overloaded-looking payload is *not* sufficient to turn a
// buffered response into a streaming retry. The 0.437.1 explicit-MIME
// precedence protects JSON and non-streaming bodies from side effects.
func TestProdex04371BoundTurnDoesNotRetryExplicitNonSSE(t *testing.T) {
	const owner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, fixture := range []struct {
		name  string
		mime  string
		input string
	}{
		{"explicit_json", "application/json", `{"stream":true,"input":[]}`},
		{"explicit_text", "text/plain", `{"stream":true,"input":[]}`},
		{"headerless_unary", "", `{"stream":false,"input":[]}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &boundOverload04371Gateway{contentType: fixture.mime}
			router, err := NewRouter(Config{
				Gateway: gateway, PreferredAccount: owner,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: owner, Home: "/synthetic", Enabled: true}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := router.affinity.rememberVerified(t.Context(), owner, affinityKeys{turn: "sticky-nonstream"}, router.now()); err != nil {
				t.Fatal(err)
			}
			response, err := router.Forward(t.Context(), proxymodel.Request{
				Path:           "/backend-api/codex/responses",
				Header:         http.Header{"X-Codex-Turn-State": []string{"sticky-nonstream"}},
				Body:           []byte(fixture.input),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			if len(gateway.owners) != 1 {
				t.Fatalf("wrongly retried non-SSE body: %v", gateway.owners)
			}
			content, err := io.ReadAll(response.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(response.Result.Prefix)+string(content), "server_is_overloaded") {
				t.Fatal("original buffered/error response was hidden")
			}
		})
	}
}
