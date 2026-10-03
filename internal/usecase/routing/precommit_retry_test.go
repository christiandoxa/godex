package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type precommitRetryReply struct {
	code      string
	transport bool
	retryUsed bool
	committed bool
	body      string
}

type precommitRetryGateway struct {
	replies  map[string]precommitRetryReply
	calls    []string
	requests []proxymodel.Request
}

func (gateway *precommitRetryGateway) Execute(_ context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	gateway.requests = append(gateway.requests, request)
	reply, ok := gateway.replies[account.ID]
	if !ok {
		return nil, fmt.Errorf("unexpected account attempt %q", account.ID)
	}
	response := &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:                io.NopCloser(strings.NewReader(reply.body)),
		FirstEventRetryUsed: reply.retryUsed,
		FirstEventCommitted: reply.committed,
	}
	if reply.code != "" || reply.transport {
		response.PrecommitFailure = &proxymodel.PrecommitFailure{Code: reply.code, Transport: reply.transport}
	}
	return response, nil
}

func TestDeepSeekFirstEventRetryRotatesOneCredentialBeforeCommit(t *testing.T) {
	accounts := []proxymodel.Account{
		{ID: "a", Home: "/a", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
		{ID: "b", Home: "/b", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
		{ID: "c", Home: "/c", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}},
	}
	for _, fixture := range []struct {
		name       string
		replies    map[string]precommitRetryReply
		wantCalls  string
		wantOwner  string
		wantFailed bool
		wantRetry  []bool
	}{
		{
			name: "transient error rotates once",
			replies: map[string]precommitRetryReply{
				"a": {code: "overloaded_error", body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
				"b": {retryUsed: true, committed: true, body: "event: response.created\ndata: {}\n\n"},
			},
			wantCalls: "a,b", wantOwner: "b", wantRetry: []bool{false, true},
		},
		{
			name: "auth error rotates credential",
			replies: map[string]precommitRetryReply{
				"a": {code: "authentication_error", body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
				"b": {retryUsed: true, committed: true, body: "event: response.created\ndata: {}\n\n"},
			},
			wantCalls: "a,b", wantOwner: "b", wantRetry: []bool{false, true},
		},
		{
			name: "read failure rotates credential",
			replies: map[string]precommitRetryReply{
				"a": {transport: true},
				"b": {retryUsed: true, committed: true, body: "event: response.created\ndata: {}\n\n"},
			},
			wantCalls: "a,b", wantOwner: "b", wantRetry: []bool{false, true},
		},
		{
			name: "not found stays terminal",
			replies: map[string]precommitRetryReply{
				"a": {code: "not_found_error", body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
			},
			wantCalls: "a", wantOwner: "a", wantFailed: true, wantRetry: []bool{false},
		},
		{
			name: "committed error does not rotate",
			replies: map[string]precommitRetryReply{
				"a": {code: "overloaded_error", retryUsed: true, committed: true, body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
			},
			wantCalls: "a", wantOwner: "a", wantFailed: true, wantRetry: []bool{false},
		},
		{
			name: "retry budget spans credentials",
			replies: map[string]precommitRetryReply{
				"a": {code: "overloaded_error", body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
				"b": {code: "overloaded_error", retryUsed: true, committed: true, body: "event: error\ndata: {\"type\":\"error\"}\n\n"},
			},
			wantCalls: "a,b", wantOwner: "b", wantFailed: true, wantRetry: []bool{false, true},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &precommitRetryGateway{replies: fixture.replies}
			router, err := NewRouter(Config{
				Gateway:          gateway,
				Accounts:         func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
				PreferredAccount: "a",
			})
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if got := strings.Join(gateway.calls, ","); got != fixture.wantCalls {
				t.Fatalf("account attempts = %q, want %q", got, fixture.wantCalls)
			}
			if exchange.Result.AccountID != fixture.wantOwner || exchange.Result.Failed != fixture.wantFailed {
				t.Fatalf("forwarded owner/failure = %q/%t", exchange.Result.AccountID, exchange.Result.Failed)
			}
			for index, want := range fixture.wantRetry {
				if gateway.requests[index].FirstEventRetryUsed != want {
					t.Fatalf("attempt %d retry budget = %t, want %t", index, gateway.requests[index].FirstEventRetryUsed, want)
				}
			}
		})
	}
}
