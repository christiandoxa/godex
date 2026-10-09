package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

type responseRecoveryReply struct {
	status            int
	contentType, body string
}

type responseRecoveryGateway struct {
	replies  []responseRecoveryReply
	requests []proxymodel.Request
	accounts []string
}

func (gateway *responseRecoveryGateway) Execute(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	index := len(gateway.requests)
	gateway.requests = append(gateway.requests, request)
	gateway.accounts = append(gateway.accounts, account.ID)
	if index >= len(gateway.replies) {
		return nil, io.EOF
	}
	reply := gateway.replies[index]
	return &proxymodel.Response{
		StatusCode: reply.status,
		Header:     http.Header{"Content-Type": []string{reply.contentType}},
		Body:       io.NopCloser(strings.NewReader(reply.body)),
	}, nil
}

func TestResponsesInvalidPreviousIDRetriesOwnedFullHistoryOnce(t *testing.T) {
	const original = `{"model":"gpt-5.6","previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"answer one"},{"type":"message","role":"user","content":"turn two"}],"client_metadata":{"session_id":"session-a","turn_id":"turn-two"}}`
	const invalid = `{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `.","param":"previous_response_id"}}`
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusBadRequest, "application/json", invalid},
		{http.StatusOK, "application/json", `{"object":"response","id":"resp-new"}`},
	}}
	router := newResponseRecoveryRouter(t, gateway, true)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(original))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a", "account-a"}) || len(gateway.requests) != 2 {
		t.Fatalf("recovery attempts = %v (%d requests)", gateway.accounts, len(gateway.requests))
	}
	var before, after map[string]any
	if err := json.Unmarshal([]byte(original), &before); err != nil {
		t.Fatal(err)
	}
	delete(before, "previous_response_id")
	if err := json.Unmarshal(gateway.requests[1].Body, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("recovery request changed full history: got %#v, want %#v", after, before)
	}
	if owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: "resp-dead"}, router.now()); err != nil || owner != "" {
		t.Fatalf("stale response owner = %q, %v", owner, err)
	}
	if owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: "resp-new"}, router.now()); err != nil || owner != "account-a" {
		t.Fatalf("recovered response owner = %q, %v", owner, err)
	}
}

func TestResponsesInvalidPreviousIDClearsDurableBindingAndTurnState(t *testing.T) {
	const accountID = "0123456789abcdef0123456789abcdef"
	const body = `{"model":"gpt-5.6","previous_response_id":"resp-dead","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}],"client_metadata":{"session_id":"session-a"}}`
	const invalid = `{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusBadRequest, "application/json", invalid},
		{http.StatusOK, "application/json", `{"object":"response","id":"resp-new"}`},
	}}
	now := time.Unix(100, 0)
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	store := routingrepo.NewStore(t.TempDir())
	router, err := NewRouter(Config{
		Gateway: gateway, Bindings: store, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: accountID, Home: home, Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.remember(context.Background(), accountID, affinityKeys{
		previous: "resp-dead", session: "session-a",
	}, now); err != nil {
		t.Fatal(err)
	}
	turnKey := affinityDigest("previous", "resp-dead")
	router.affinity.rememberResponseTurnStateForHome(
		context.Background(), "resp-dead", accountID, home, "turn-dead", now,
	)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
	if err != nil {
		t.Fatalf("forward with durable stale response: %v (attempts %d)", err, len(gateway.requests))
	}
	defer exchange.Close()
	if len(gateway.requests) != 2 || gateway.requests[1].Header.Get("x-codex-turn-state") != "turn-dead" {
		t.Fatalf("recovery request count/turn state = %d / %q", len(gateway.requests), gateway.requests[1].Header.Get("x-codex-turn-state"))
	}
	bindings, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldBinding, sessionBinding := false, false
	for _, binding := range bindings {
		oldBinding = oldBinding || binding.Key == turnKey
		sessionBinding = sessionBinding || binding.Key == affinityDigest("session", "session-a") && binding.AccountID == accountID
	}
	if oldBinding || !sessionBinding {
		t.Fatalf("durable response/session bindings = %#v", bindings)
	}
	if _, err := os.Lstat(filepath.Join(home, ".godex-turn-state", turnKey+".json")); !os.IsNotExist(err) {
		t.Fatalf("stale turn-state sidecar remains: %v", err)
	}
}

func TestResponsesInvalidPreviousIDDoesNotReplayMessageFollowup(t *testing.T) {
	const invalid = `{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"assistant","content":"answer one"},{"type":"message","role":"user","content":"turn two"}],"client_metadata":{"session_id":"session-a"}}`
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusBadRequest, "application/json", invalid},
	}}
	router := newResponseRecoveryRouter(t, gateway, true)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	replayed, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil || string(replayed) != invalid {
		t.Fatalf("original invalid response = %q, %v", replayed, err)
	}
	if len(gateway.requests) != 1 {
		t.Fatalf("message follow-up attempts = %d, want 1", len(gateway.requests))
	}
	if owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: "resp-dead"}, router.now()); err != nil || owner != "" {
		t.Fatalf("stale response owner = %q, %v", owner, err)
	}
}

func TestResponsesInvalidPreviousIDSSERecoversOnSameOwnerOnce(t *testing.T) {
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"answer one"},{"type":"message","role":"user","content":"turn two"}],"client_metadata":{"session_id":"session-a"}}`
	const invalid = "event: error\ndata: {\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}\n\n"
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusOK, "text/event-stream", invalid},
		{http.StatusOK, "text/event-stream", "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-new\"}}\n\n"},
	}}
	router := newResponseRecoveryRouter(t, gateway, true)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a", "account-a"}) || len(gateway.requests) != 2 {
		t.Fatalf("SSE recovery attempts = %v (%d requests)", gateway.accounts, len(gateway.requests))
	}
	if strings.Contains(string(gateway.requests[1].Body), "previous_response_id") {
		t.Fatalf("SSE recovery retained stale id: %s", gateway.requests[1].Body)
	}
}

func TestResponsesInvalidPreviousIDHeaderlessRequestedSSERecovers(t *testing.T) {
	const invalid = "event: error\ndata: {\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}\n\n"
	const recovered = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-new\"}}\n\n"
	for _, fixture := range []struct {
		name         string
		stream       bool
		contentType  string
		wantRecovery bool
	}{
		{name: "requested headerless SSE", stream: true, wantRecovery: true},
		{name: "requested explicit JSON", stream: true, contentType: "application/json"},
		{name: "unary headerless", stream: false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := `{"stream":false,"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"answer one"},{"type":"message","role":"user","content":"turn two"}],"client_metadata":{"session_id":"session-a"}}`
			if fixture.stream {
				body = strings.Replace(body, `"stream":false`, `"stream":true`, 1)
			}
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusOK, fixture.contentType, invalid},
				{http.StatusOK, fixture.contentType, recovered},
			}}
			router := newResponseRecoveryRouter(t, gateway, true)
			exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			wantRequests := 1
			if fixture.wantRecovery {
				wantRequests = 2
			}
			if len(gateway.requests) != wantRequests {
				t.Fatalf("attempts = %d, want %d", len(gateway.requests), wantRequests)
			}
			if fixture.wantRecovery && strings.Contains(string(gateway.requests[1].Body), "previous_response_id") {
				t.Fatalf("headerless SSE recovery retained stale id: %s", gateway.requests[1].Body)
			}
		})
	}
}

func TestResponsesInvalidPreviousIDRetriesAtMostOnce(t *testing.T) {
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user"},{"type":"message","role":"assistant"},{"type":"message","role":"user"}],"session_id":"session-a"}`
	const invalid = `{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusBadRequest, "application/json", invalid},
		{http.StatusBadRequest, "application/json", invalid},
		{http.StatusOK, "application/json", `{"id":"must-not-reach"}`},
	}}
	router := newResponseRecoveryRouter(t, gateway, true)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if len(gateway.requests) != 2 || strings.Contains(string(gateway.requests[1].Body), "previous_response_id") {
		t.Fatalf("recovery attempts/body = %d / %s", len(gateway.requests), gateway.requests[1].Body)
	}
}

func TestResponsesInvalidPreviousIDNeedsOwnedBindingAndExactError(t *testing.T) {
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"answer one"},{"type":"message","role":"user","content":"turn two"}],"client_metadata":{"session_id":"session-a"}}`
	for _, fixture := range []struct {
		name          string
		ownedResponse bool
		responseBody  string
	}{
		{name: "session owner alone", responseBody: `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`},
		{name: "near match", ownedResponse: true, responseBody: `{"error":{"type":"invalid_request_error","message":"Invalid previous_response_id."}}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{{
				http.StatusBadRequest, "application/json", fixture.responseBody,
			}}}
			router := newResponseRecoveryRouter(t, gateway, fixture.ownedResponse)
			if !fixture.ownedResponse {
				if err := router.affinity.remember(context.Background(), "account-a", affinityKeys{session: "session-a"}, router.now()); err != nil {
					t.Fatal(err)
				}
			}
			exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(body))
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if len(gateway.requests) != 1 {
				t.Fatalf("attempts = %d, want 1", len(gateway.requests))
			}
			if fixture.ownedResponse {
				if owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: "resp-dead"}, router.now()); err != nil || owner != "account-a" {
					t.Fatalf("near-match response owner = %q, %v", owner, err)
				}
			}
		})
	}
}

func TestReconstructableFullHistoryMatchesCodexRequestShapes(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		input []json.RawMessage
		want  bool
	}{
		{
			name: "completed turn before current user",
			input: []json.RawMessage{
				json.RawMessage(`{"role":"user"}`),
				json.RawMessage(`{"role":"assistant"}`),
				json.RawMessage(`{"role":"user"}`),
			},
			want: true,
		},
		{
			name: "compaction before remaining input",
			input: []json.RawMessage{
				json.RawMessage(`{"type":"compaction"}`),
				json.RawMessage(`{"role":"user"}`),
			},
			want: true,
		},
		{
			name: "message followup has no replayable history",
			input: []json.RawMessage{
				json.RawMessage(`{"role":"assistant"}`),
				json.RawMessage(`{"role":"user"}`),
			},
		},
		{
			name: "assistant must have a later item",
			input: []json.RawMessage{
				json.RawMessage(`{"role":"user"}`),
				json.RawMessage(`{"role":"assistant"}`),
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := reconstructableFullHistory(fixture.input); got != fixture.want {
				t.Fatalf("reconstructable = %t, want %t", got, fixture.want)
			}
		})
	}
}

func newResponseRecoveryRouter(t *testing.T, gateway *responseRecoveryGateway, bindResponse bool) *Router {
	t.Helper()
	now := time.Unix(100, 0)
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a", Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/profile", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bindResponse {
		if err := router.affinity.remember(context.Background(), "account-a", affinityKeys{
			previous: "resp-dead", session: "session-a",
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	return router
}

func responsesRecoveryRequest(body string) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/codex/responses",
		Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(body),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6"},
	}
}
