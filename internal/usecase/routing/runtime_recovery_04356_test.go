package routing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type prodex04356Step struct {
	account   string
	status    int
	body      string
	committed bool
	err       error
}

type prodex04356Gateway struct {
	steps    []prodex04356Step
	calls    []string
	requests []proxymodel.Request
}

func (gateway *prodex04356Gateway) Execute(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	index := len(gateway.calls)
	gateway.calls = append(gateway.calls, account.ID)
	captured := request
	captured.Body = append([]byte(nil), request.Body...)
	if request.Header != nil {
		captured.Header = request.Header.Clone()
	}
	gateway.requests = append(gateway.requests, captured)
	if index >= len(gateway.steps) {
		return nil, errors.New("unexpected extra upstream attempt")
	}
	step := gateway.steps[index]
	if step.account != "" && step.account != account.ID {
		return nil, errors.New("unexpected upstream account " + account.ID + ", want " + step.account)
	}
	if step.err != nil {
		return nil, step.err
	}
	status := step.status
	if status == 0 {
		status = http.StatusOK
	}
	return &proxymodel.Response{
		StatusCode:          status,
		Header:              http.Header{"Content-Type": []string{"application/json"}},
		Body:                io.NopCloser(strings.NewReader(step.body)),
		FirstEventCommitted: step.committed,
	}, nil
}

func prodex04356Accounts() []proxymodel.Account {
	return []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1, Provider: proxymodel.Provider{Kind: "openai"}},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2, Provider: proxymodel.Provider{Kind: "openai"}},
	}
}

func newProdex04356Router(t *testing.T, gateway gateway, accounts []proxymodel.Account) *Router {
	t.Helper()
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", MaxInspectBytes: 64 << 10,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
		Wait: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestProdex04356Generic429RotatesBeforeCommitOnly(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		committed bool
		wantCalls string
		wantOwner string
		wantCode  int
	}{
		{name: "precommit rotates", wantCalls: "account-a,account-b", wantOwner: "account-b", wantCode: http.StatusOK},
		{name: "committed passes through", committed: true, wantCalls: "account-a", wantOwner: "account-a", wantCode: http.StatusTooManyRequests},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			steps := []prodex04356Step{
				{account: "account-a", status: http.StatusTooManyRequests, body: `{"error":{"message":"Too Many Requests"}}`, committed: fixture.committed},
			}
			if !fixture.committed {
				steps = append(steps, prodex04356Step{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`})
			}
			gateway := &prodex04356Gateway{steps: steps}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Header:         make(http.Header),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if got := strings.Join(gateway.calls, ","); got != fixture.wantCalls ||
				exchange.Result.AccountID != fixture.wantOwner ||
				exchange.Result.Response.StatusCode != fixture.wantCode {
				t.Fatalf("calls/owner/status = %s/%s/%d, want %s/%s/%d",
					got, exchange.Result.AccountID, exchange.Result.Response.StatusCode,
					fixture.wantCalls, fixture.wantOwner, fixture.wantCode)
			}
		})
	}
}

func TestProdex04356Generic429NonRetryableMarkerPassesThrough(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{{
		account: "account-a", status: http.StatusTooManyRequests,
		body: `{"error":{"type":"invalid_request_error","message":"bad request"}}`,
	}}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a" ||
		exchange.Result.Response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("calls/status = %s/%d, want account-a/429", got, exchange.Result.Response.StatusCode)
	}
}

func TestProdex04356StandardBoundSessionRetryableFailuresRotateAndRebind(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "generic 429", status: 429, body: `{"error":{"message":"Too Many Requests"}}`},
		{name: "quota", status: 429, body: `{"error":{"code":"insufficient_quota"}}`},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "overload", status: 503, body: `{"error":{"code":"server_is_overloaded"}}`},
		{name: "profile unavailable", status: 403, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{name: "auth", status: 401, body: `{"error":{"code":"authentication_error"}}`},
		{name: "transport", err: errors.New("connection reset by peer")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{account: "account-a", status: fixture.status, body: fixture.body, err: fixture.err},
				{account: "account-b", status: http.StatusOK, body: `{"ok":true}`},
			}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{session: "sess-bound"}, router.now()); err != nil {
				t.Fatal(err)
			}
			request := proxymodel.Request{
				Header:         http.Header{"X-Codex-Session-Id": []string{"sess-bound"}},
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard},
			}
			exchange, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			owner, ownerErr := router.affinity.owner(t.Context(), affinityKeys{session: "sess-bound"}, router.now())
			if ownerErr != nil {
				t.Fatal(ownerErr)
			}
			if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
				exchange.Result.AccountID != "account-b" || exchange.Result.Response.StatusCode != http.StatusOK ||
				owner != "account-b" {
				t.Fatalf("calls/result/rebound = %s/%s/%d/%s", got, exchange.Result.AccountID, exchange.Result.Response.StatusCode, owner)
			}
		})
	}
}

func TestProdex04356ResponsesHardAffinityFailureSignalsFullContextReplayWithoutSession(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: http.StatusTooManyRequests, body: `{"error":{"message":"Too Many Requests"}}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-replayed"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-owner"}, router.now()); err != nil {
		t.Fatal(err)
	}

	continuation, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Header: make(http.Header),
		Body:           []byte(`{"previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(continuation.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = continuation.Close()
	owner, ownerErr := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-owner"}, router.now())
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if got := strings.Join(gateway.calls, ","); got != "account-a" ||
		continuation.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "previous_response_not_found") ||
		!strings.Contains(string(body), "Previous response was not found. Retrying the full request.") ||
		owner != "" {
		t.Fatalf("signal calls/status/body/owner = %s/%d/%s/%q", got, continuation.Result.Response.StatusCode, body, owner)
	}

	replay, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Header: make(http.Header),
		Body:           []byte(`{"input":[{"role":"user","content":"original prompt"},{"role":"assistant","content":"previous answer"},{"role":"user","content":"continue"}]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		replay.Result.AccountID != "account-b" || replay.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("replay calls/owner/status = %s/%s/%d", got, replay.Result.AccountID, replay.Result.Response.StatusCode)
	}
}

func TestProdex04356HTTPPreviousAndSessionRetryableFailureSignalsFullContextBeforeFallback(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body string
		step prodex04356Step
	}{
		{
			name: "message quota",
			body: "{\"previous_response_id\":\"resp-session\",\"session_id\":\"sess-replayable\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"continue after quota pressure\"}]}",
			step: prodex04356Step{account: "account-a", status: http.StatusTooManyRequests, body: "{\"error\":{\"code\":\"insufficient_quota\"}}"},
		},
		{
			name: "tool output quota",
			body: "{\"previous_response_id\":\"resp-session\",\"session_id\":\"sess-replayable\",\"input\":[{\"type\":\"function_call_output\",\"call_id\":\"call-1\",\"output\":\"ok\"}]}",
			step: prodex04356Step{account: "account-a", status: http.StatusTooManyRequests, body: "{\"error\":{\"code\":\"insufficient_quota\"}}"},
		},
		{
			name: "message transport",
			body: "{\"previous_response_id\":\"resp-session\",\"session_id\":\"sess-replayable\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"continue after transport failure\"}]}",
			step: prodex04356Step{account: "account-a", err: errors.New("connection reset by peer")},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				fixture.step,
				{account: "account-b", status: http.StatusOK, body: "{\"id\":\"must-not-run\"}"},
			}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			if err := router.affinity.remember(
				t.Context(), "account-a", affinityKeys{previous: "resp-session"}, router.now(),
			); err != nil {
				t.Fatal(err)
			}

			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Method:         http.MethodPost,
				Header:         make(http.Header),
				Body:           []byte(fixture.body),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			payload, readErr := io.ReadAll(exchange.Result.Response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			_ = exchange.Close()

			previousOwner, previousErr := router.affinity.owner(
				t.Context(), affinityKeys{previous: "resp-session"}, router.now(),
			)
			if previousErr != nil {
				t.Fatal(previousErr)
			}
			sessionOwner, sessionErr := router.affinity.owner(
				t.Context(), affinityKeys{session: "sess-replayable"}, router.now(),
			)
			if sessionErr != nil {
				t.Fatal(sessionErr)
			}
			capturedBody := ""
			if len(gateway.requests) > 0 {
				capturedBody = string(gateway.requests[0].Body)
			}
			if got := strings.Join(gateway.calls, ","); got != "account-a" ||
				exchange.Result.Response.StatusCode != http.StatusBadRequest ||
				!strings.Contains(string(payload), "previous_response_not_found") ||
				previousOwner != "" || sessionOwner != "" ||
				len(gateway.requests) != 1 || capturedBody != fixture.body {
				t.Fatalf(
					"calls/status/body/owners/requests = %s/%d/%s/%q,%q/%d:%q",
					got, exchange.Result.Response.StatusCode, payload,
					previousOwner, sessionOwner, len(gateway.requests), capturedBody,
				)
			}
		})
	}
}

func TestProdex04356ResponsesHardAffinityRetryableMatrixSignalsFullContextWithoutSession(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "quota", status: 429, body: `{"error":{"code":"insufficient_quota"}}`},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "overload", status: 503, body: `{"error":{"code":"server_is_overloaded"}}`},
		{name: "auth", status: 401, body: `{"error":{"code":"authentication_error"}}`},
		{name: "profile unavailable", status: 403, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{name: "transport", err: errors.New("connection reset by peer")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{account: "account-a", status: fixture.status, body: fixture.body, err: fixture.err},
			}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			if err := router.affinity.remember(
				t.Context(), "account-a", affinityKeys{previous: "resp-matrix"}, router.now(),
			); err != nil {
				t.Fatal(err)
			}

			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Header: make(http.Header),
				Body:           []byte(`{"previous_response_id":"resp-matrix","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			payload, readErr := io.ReadAll(exchange.Result.Response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			_ = exchange.Close()
			owner, ownerErr := router.affinity.owner(
				t.Context(), affinityKeys{previous: "resp-matrix"}, router.now(),
			)
			if ownerErr != nil {
				t.Fatal(ownerErr)
			}
			if got := strings.Join(gateway.calls, ","); got != "account-a" ||
				exchange.Result.Response.StatusCode != http.StatusBadRequest ||
				!strings.Contains(string(payload), "previous_response_not_found") ||
				!strings.Contains(string(payload), "Previous response was not found. Retrying the full request.") ||
				owner != "" {
				t.Fatalf("signal calls/status/body/owner = %s/%d/%s/%q",
					got, exchange.Result.Response.StatusCode, payload, owner)
			}
		})
	}
}

func TestProdex04356ResponsesHardAffinityWithoutFallbackKeepsUpstreamFailure(t *testing.T) {
	accounts := prodex04356Accounts()[:1]
	gateway := &prodex04356Gateway{steps: []prodex04356Step{{
		account: "account-a", status: http.StatusTooManyRequests,
		body: `{"error":{"code":"insufficient_quota","message":"Quota exhausted"}}`,
	}}}
	router := newProdex04356Router(t, gateway, accounts)
	if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-owner"}, router.now()); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		Body:           []byte(`{"previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	owner, ownerErr := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-owner"}, router.now())
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if got := strings.Join(gateway.calls, ","); got != "account-a" ||
		exchange.Result.Response.StatusCode != http.StatusTooManyRequests ||
		owner != "account-a" {
		t.Fatalf("calls/status/owner = %s/%d/%q", got, exchange.Result.Response.StatusCode, owner)
	}
}

func TestProdex04356CompactPreviousAffinityRetryableFailuresSignalFullContext(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "quota", status: 429, body: `{"error":{"code":"insufficient_quota"}}`},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "overload", status: 503, body: `{"error":{"code":"server_is_overloaded"}}`},
		{name: "auth", status: 401, body: `{"error":{"code":"authentication_error"}}`},
		{name: "profile unavailable", status: 403, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{name: "transport", err: errors.New("connection reset by peer")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{account: "account-a", status: fixture.status, body: fixture.body, err: fixture.err},
				{account: "account-b", status: http.StatusOK, body: `{"id":"compact-replay"}`},
			}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			if err := router.affinity.remember(
				t.Context(), "account-a", affinityKeys{previous: "resp-compact"}, router.now(),
			); err != nil {
				t.Fatal(err)
			}
			continuation, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Header: make(http.Header),
				Body:           []byte(`{"previous_response_id":"resp-compact","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
			})
			if err != nil {
				t.Fatal(err)
			}
			payload, readErr := io.ReadAll(continuation.Result.Response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			_ = continuation.Close()
			owner, ownerErr := router.affinity.owner(
				t.Context(), affinityKeys{previous: "resp-compact"}, router.now(),
			)
			if ownerErr != nil {
				t.Fatal(ownerErr)
			}
			if got := strings.Join(gateway.calls, ","); got != "account-a" ||
				continuation.Result.Response.StatusCode != http.StatusBadRequest ||
				!strings.Contains(string(payload), "previous_response_not_found") ||
				owner != "" {
				t.Fatalf("compact signal calls/status/body/owner = %s/%d/%s/%q",
					got, continuation.Result.Response.StatusCode, payload, owner)
			}

			replay, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Header: make(http.Header),
				Body:           []byte(`{"input":[{"role":"user","content":"full compact context"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer replay.Close()
			if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
				replay.Result.AccountID != "account-b" {
				t.Fatalf("compact replay calls/owner = %s/%s", got, replay.Result.AccountID)
			}
		})
	}
}

func TestProdex04356CompactSessionRetryableFailuresRotateAndRebind(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "quota", status: 429, body: `{"error":{"code":"insufficient_quota"}}`},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "overload", status: 503, body: `{"error":{"code":"server_is_overloaded"}}`},
		{name: "auth", status: 401, body: `{"error":{"code":"authentication_error"}}`},
		{name: "profile unavailable", status: 403, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{name: "transport", err: errors.New("connection reset by peer")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{account: "account-a", status: fixture.status, body: fixture.body, err: fixture.err},
				{account: "account-b", status: http.StatusOK, body: `{"id":"compact-b"}`},
			}}
			router := newProdex04356Router(t, gateway, prodex04356Accounts())
			if err := router.affinity.remember(
				t.Context(), "account-a", affinityKeys{session: "compact-session"}, router.now(),
			); err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Method:         http.MethodPost,
				Header:         http.Header{"X-Codex-Session-Id": []string{"compact-session"}},
				Body:           []byte(`{"input":[{"role":"user","content":"compact"}]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			owner, ownerErr := router.affinity.owner(
				t.Context(), affinityKeys{session: "compact-session"}, router.now(),
			)
			if ownerErr != nil {
				t.Fatal(ownerErr)
			}
			if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
				exchange.Result.AccountID != "account-b" || owner != "account-b" {
				t.Fatalf("compact session calls/result/owner = %s/%s/%s",
					got, exchange.Result.AccountID, owner)
			}
		})
	}
}

func TestProdex04356CompactRateLimitPoolRecoveryRetriesUntilSuccess(t *testing.T) {
	now := time.Unix(9_000_000, 0)
	accounts := prodex04356Accounts()
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{account: "account-b", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{account: "account-a", status: http.StatusOK, body: `{"id":"compact-recovered"}`},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", MaxInspectBytes: 64 << 10,
		Now: func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error {
			now = now.Add(delay + time.Second)
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Header: make(http.Header),
		Body:           []byte(`{"input":[{"role":"user","content":"compact"}]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b,account-a" ||
		exchange.Result.Response.StatusCode != http.StatusOK ||
		exchange.Result.AccountID != "account-a" {
		t.Fatalf("compact recovery calls/status/owner = %s/%d/%s",
			got, exchange.Result.Response.StatusCode, exchange.Result.AccountID)
	}
}

func TestProdex04356CanonicalUsageLimit429ClassifiesAsQuota(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: 429, body: `{"error":{"message":"You've hit your usage limit. Upgrade to Pro or try again later."},"status":429}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		exchange.Result.AccountID != "account-b" || !router.quotaBlockedAccount("account-a") {
		t.Fatalf("canonical quota calls/owner/blocked = %s/%s/%t",
			got, exchange.Result.AccountID, router.quotaBlockedAccount("account-a"))
	}
}

func TestProdex04356NonAuthoritativeUsageLimit429RemainsRateLimited(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: 429, body: `{"error":{"message":"The usage limit has been reached"}}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		exchange.Result.AccountID != "account-b" || router.quotaBlockedAccount("account-a") {
		t.Fatalf("generic usage calls/owner/blocked = %s/%s/%t",
			got, exchange.Result.AccountID, router.quotaBlockedAccount("account-a"))
	}
}

func TestProdex04356HTTP429RateCodeWinsQuotaCode(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: 429, body: `{"error":{"code":"insufficient_quota","type":"rate_limit_exceeded","message":"multiple signals"}}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		router.quotaBlockedAccount("account-a") {
		t.Fatalf("429 precedence calls/quota = %s/%t", got, router.quotaBlockedAccount("account-a"))
	}
}

func TestProdex04356HTTP429DeactivatedWorkspaceFallsBackToGenericRate(t *testing.T) {
	gateway := &prodex04356Gateway{steps: []prodex04356Step{
		{account: "account-a", status: 429, body: `{"detail":{"code":"deactivated_workspace"}}`},
		{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`},
	}}
	router := newProdex04356Router(t, gateway, prodex04356Accounts())
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Header:         make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		router.quotaBlockedAccount("account-a") {
		t.Fatalf("429 deactivated calls/quota = %s/%t", got, router.quotaBlockedAccount("account-a"))
	}
}

func TestProdex04356ProfileStatusQuotaCodeCorpusRotates(t *testing.T) {
	codes := []string{
		"insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted",
		"usage_limit_reached", "usage_not_included", "workspace_member_credits_depleted",
	}
	for _, status := range []int{http.StatusPaymentRequired, http.StatusForbidden} {
		for _, code := range codes {
			t.Run(fmt.Sprintf("%d/%s", status, code), func(t *testing.T) {
				gateway := &prodex04356Gateway{steps: []prodex04356Step{
					{account: "account-a", status: status, body: fmt.Sprintf(`{"error":{"reason":%q,"message":"quota"}}`, code)},
					{account: "account-b", status: http.StatusOK, body: `{"id":"resp-b"}`},
				}}
				router := newProdex04356Router(t, gateway, prodex04356Accounts())
				exchange, err := router.Forward(t.Context(), proxymodel.Request{
					Header:         make(http.Header),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer exchange.Close()
				if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
					exchange.Result.AccountID != "account-b" || !router.quotaBlockedAccount("account-a") {
					t.Fatalf("calls/owner/quota = %s/%s/%t", got, exchange.Result.AccountID, router.quotaBlockedAccount("account-a"))
				}
			})
		}
	}
}

func TestProdex04356ProfileStatusRateCodeIsNotQuota(t *testing.T) {
	if isQuotaResponse([]byte(`{"error":{"reason":"rate_limit_exceeded"}}`)) {
		t.Fatal("rate_limit_exceeded must not be classified as quota on profile status paths")
	}
}

func TestProdex04361ScalarErrorQuotaCodeIsQuota(t *testing.T) {
	for _, code := range []string{
		"insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted",
		"usage_limit_reached", "usage_not_included", "workspace_member_credits_depleted",
	} {
		if !isQuotaResponse([]byte(fmt.Sprintf(`{"error":%q}`, code))) {
			t.Errorf("scalar error code was not classified as quota: %q", code)
		}
	}
}

func TestProdex04356ProfileStatusUsageMessageCorpusIsQuota(t *testing.T) {
	messages := []string{
		"You've hit your usage limit.",
		"You have hit your usage limit.",
		"The usage limit has been reached.",
		"Usage limit has been reached.",
		"Usage limit reached; try again at 5:08 PM.",
		"Usage limit reached; request to your admin for more capacity.",
		"Usage limit reached; get more access now.",
		"Your workspace is out of credits.",
		"You are out of credits. Ask your workspace owner to refill.",
	}
	for _, message := range messages {
		if !openAIWorkspaceQuotaResponse([]byte(message)) {
			t.Errorf("message was not classified as quota: %q", message)
		}
	}
	if openAIWorkspaceQuotaResponse([]byte("You hit your usage limit.")) {
		t.Fatal("bare 'you hit your usage limit' must not classify 402/403 workspace recovery")
	}
}

func TestProdex04356HTTP429StructuredSSESignalsKeepTaggedPrecedence(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		body      string
		wantQuota bool
	}{
		{
			name: "rate code",
			body: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"Rate limit exceeded\"}}}\n\n",
		},
		{
			name:      "quota code",
			body:      "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"insufficient_quota\",\"message\":\"Quota exhausted\"}}}\n\n",
			wantQuota: true,
		},
		{
			name: "rate wins quota",
			body: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"insufficient_quota\",\"type\":\"rate_limit_exceeded\",\"message\":\"multiple signals\"}}}\n\n",
		},
		{
			name: "message only stays generic rate",
			body: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"The usage limit has been reached\"}}}\n\n",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			classification := openAI429Classification([]byte(fixture.body))
			if gotQuota := classification.Class == providerentity.ErrorQuota; gotQuota != fixture.wantQuota {
				t.Fatalf("classification = %#v, wantQuota=%t", classification, fixture.wantQuota)
			}
			if !fixture.wantQuota && classification.Class != providerentity.ErrorRateLimit {
				t.Fatalf("classification = %#v, want rate limit", classification)
			}
		})
	}
}

func TestProdex04361HTTP429RateLimitHeaderOverridesBody(t *testing.T) {
	router := newProdex04356Router(t, &prodex04356Gateway{}, prodex04356Accounts())
	for _, fixture := range []struct {
		name      string
		header    string
		wantQuota bool
	}{
		{name: "rate", header: "rate_limit_reached"},
		{name: "workspace member", header: "workspace_member_credits_depleted", wantQuota: true},
		{name: "workspace owner usage", header: "workspace_owner_usage_limit_reached", wantQuota: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"X-Codex-Rate-Limit-Reached-Type": []string{fixture.header},
				},
				Body: io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded"}}`)),
			}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			pending.close()
			if outcome.kind != responseRetry || outcome.quota != fixture.wantQuota {
				t.Fatalf("header %q outcome = %#v, want quota=%t", fixture.header, outcome, fixture.wantQuota)
			}
		})
	}
}
