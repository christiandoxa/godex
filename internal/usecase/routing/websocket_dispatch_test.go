package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketDispatchGateway struct {
	mu             sync.Mutex
	standardCalls  int
	websocketCalls int
	requests       []proxymodel.Request
	accounts       []string
	responses      []*proxymodel.Response
}

func (gateway *websocketDispatchGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.standardCalls++
	gateway.mu.Unlock()
	return nil, &proxymodel.Error{StatusCode: http.StatusInternalServerError, Message: "standard execute must not handle websocket message"}
}

func (gateway *websocketDispatchGateway) ExecuteWebSocketMessage(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.websocketCalls++
	gateway.requests = append(gateway.requests, request)
	gateway.accounts = append(gateway.accounts, account.ID)
	if len(gateway.responses) == 0 {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "missing websocket fixture"}
	}
	response := gateway.responses[0]
	gateway.responses = gateway.responses[1:]
	return response, nil
}

type websocketDispatchBody struct {
	value  string
	reads  int
	closed bool
}

func (body *websocketDispatchBody) Read(buffer []byte) (int, error) {
	body.reads++
	if body.value == "" {
		return 0, io.EOF
	}
	count := copy(buffer, body.value)
	body.value = body.value[count:]
	if body.value == "" {
		return count, io.EOF
	}
	return count, nil
}

func (body *websocketDispatchBody) Close() error {
	body.closed = true
	return nil
}

func websocketDispatchRequest(body string, sessionID uint64) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodGet,
		Path:   "/backend-api/codex/responses",
		Header: http.Header{
			"Upgrade":    {"websocket"},
			"Connection": {"Upgrade"},
		},
		Body:               []byte(body),
		WebSocketMessage:   true,
		WebSocketSessionID: sessionID,
	}
}

func TestWebSocketMessageDispatchBindsCommittedResponseWithoutReadingBody(t *testing.T) {
	firstBody := &websocketDispatchBody{value: "first-frame"}
	secondBody := &websocketDispatchBody{value: "second-frame"}
	thirdBody := &websocketDispatchBody{value: "third-frame"}
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: firstBody,
			WebSocketFrames: true, FirstEventCommitted: true,
			WebSocketResponseID: "resp-1", WebSocketTurnState: "turn-1",
		},
		{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: secondBody,
			WebSocketFrames: true, FirstEventCommitted: true,
			WebSocketResponseID: "resp-2", WebSocketTurnState: "turn-2",
		},
		{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: thirdBody,
			WebSocketFrames: true, FirstEventCommitted: true,
			WebSocketResponseID: "resp-3", WebSocketTurnState: "turn-3",
		},
	}}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: "account-a",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 41))
	if err != nil {
		t.Fatal(err)
	}
	if firstBody.reads != 0 {
		t.Fatalf("routing consumed committed websocket body before caller: reads=%d", firstBody.reads)
	}
	if first.Result.AccountID != "account-a" {
		t.Fatalf("first owner = %q", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","previous_response_id":"resp-1"}`, 42),
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.Result.AccountID != "account-a" {
		_ = second.Close()
		t.Fatalf("previous-response owner = %q", second.Result.AccountID)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	third, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","x-codex-turn-state":"turn-1"}`, 43),
	)
	if err != nil {
		t.Fatal(err)
	}
	if third.Result.AccountID != "account-a" {
		_ = third.Close()
		t.Fatalf("turn-state owner = %q", third.Result.AccountID)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if gateway.standardCalls != 0 || gateway.websocketCalls != 3 {
		t.Fatalf("standard/websocket calls = %d/%d", gateway.standardCalls, gateway.websocketCalls)
	}
	if strings.Join(gateway.accounts, ",") != "account-a,account-a,account-a" {
		t.Fatalf("account trace = %q", strings.Join(gateway.accounts, ","))
	}
	if !gateway.requests[0].WebSocketPolicy.PromoteCommittedProfile ||
		gateway.requests[0].WebSocketPolicy.RequestPreviousResponse ||
		gateway.requests[0].WebSocketPolicy.RequestTurnState {
		t.Fatalf("fresh policy = %#v", gateway.requests[0].WebSocketPolicy)
	}
	if gateway.requests[1].WebSocketPolicy.PromoteCommittedProfile ||
		!gateway.requests[1].WebSocketPolicy.RequestPreviousResponse {
		t.Fatalf("previous-response policy = %#v", gateway.requests[1].WebSocketPolicy)
	}
	if gateway.requests[2].WebSocketPolicy.PromoteCommittedProfile ||
		!gateway.requests[2].WebSocketPolicy.RequestTurnState {
		t.Fatalf("turn-state policy = %#v", gateway.requests[2].WebSocketPolicy)
	}
}

func TestUnboundWebSocketSessionCanPromoteButDisablesTransportHoldPromotion(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(strings.NewReader("frame")),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: "resp-session",
	}}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","session_id":"session-unbound"}`, 52),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = exchange.Close()

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	policy := gateway.requests[0].WebSocketPolicy
	if !policy.PromoteCommittedProfile || !policy.RequestSession {
		t.Fatalf("session policy = %#v", policy)
	}
}

func TestNonWebSocketRequestKeepsStandardGatewayPath(t *testing.T) {
	var standardCalls int
	gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
		standardCalls++
		return &proxymodel.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp-http"}`)),
		}
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   "/backend-api/codex/responses",
		Header: make(http.Header),
		Body:   []byte(`{"input":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = exchange.Close()
	if standardCalls != 1 {
		t.Fatalf("standard calls = %d", standardCalls)
	}
}

type standardOnlyRoutingGateway struct {
	execute func() *proxymodel.Response
}

func (gateway *standardOnlyRoutingGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return gateway.execute(), nil
}
