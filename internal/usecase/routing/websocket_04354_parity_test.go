package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type prodex04354WebSocketGateway struct {
	responses []*proxymodel.Response
	accounts  []string
}

func (gateway *prodex04354WebSocketGateway) Execute(
	context.Context,
	proxymodel.Request,
	proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, errors.New("standard execute must not handle websocket messages")
}

func (gateway *prodex04354WebSocketGateway) ExecuteWebSocketMessage(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.accounts = append(gateway.accounts, account.ID)
	if len(gateway.responses) == 0 {
		return &proxymodel.Response{
			StatusCode:          http.StatusOK,
			Header:              make(http.Header),
			Body:                io.NopCloser(strings.NewReader("frame")),
			WebSocketFrames:     true,
			FirstEventCommitted: true,
		}, nil
	}
	response := gateway.responses[0]
	gateway.responses = gateway.responses[1:]
	return response, nil
}

func prodex04354WebSocketRequest(body string, sessionID uint64) proxymodel.Request {
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

func TestProdex04354FreshWebSocketWaitsWithoutTransientFailureFlag(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	loads := 0
	gateway := &prodex04354WebSocketGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			loads++
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", time.Second)

	exchange, err := router.Forward(
		context.Background(),
		prodex04354WebSocketRequest(`{"type":"response.create","response":{}}`, 81),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" ||
		len(gateway.accounts) != 1 || gateway.accounts[0] != "account-a" ||
		len(waits) != 1 || loads < 2 {
		t.Fatalf(
			"owner/accounts/waits/loads = %q/%v/%v/%d",
			exchange.Result.AccountID, gateway.accounts, waits, loads,
		)
	}
}

func TestProdex04354FreshWebSocketReselectsAfterRecoveryWait(t *testing.T) {
	now := time.Unix(200, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true},
		{ID: "account-b", Home: "/b", Enabled: false},
	}
	gateway := &prodex04354WebSocketGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			now = now.Add(delay)
			accounts[0].Enabled = false
			accounts[1].Enabled = true
			return ctx.Err()
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", time.Second)

	exchange, err := router.Forward(
		context.Background(),
		prodex04354WebSocketRequest(`{"type":"response.create","response":{}}`, 82),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" ||
		len(gateway.accounts) != 1 || gateway.accounts[0] != "account-b" {
		t.Fatalf("owner/accounts = %q/%v", exchange.Result.AccountID, gateway.accounts)
	}
}

func TestProdex04354WebSocketHardContinuationDoesNotRotateDuringBackoff(t *testing.T) {
	now := time.Unix(300, 0)
	gateway := &prodex04354WebSocketGateway{responses: []*proxymodel.Response{{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(strings.NewReader("first")),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: "resp-owner",
	}}}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: "account-a",
		Now:              func() time.Time { return now },
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

	first, err := router.Forward(
		context.Background(),
		prodex04354WebSocketRequest(
			`{"type":"response.create","session_id":"session-a","response":{}}`,
			83,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	owner := first.Result.AccountID
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount(owner, time.Second)

	_, err = router.Forward(
		context.Background(),
		prodex04354WebSocketRequest(
			`{"type":"response.create","session_id":"session-a","previous_response_id":"resp-owner","response":{"previous_response_id":"resp-owner"}}`,
			84,
		),
	)
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("hard continuation error = %v", err)
	}
	if strings.Join(gateway.accounts, ",") != owner {
		t.Fatalf("hard continuation rotated profiles: owner=%q attempts=%v", owner, gateway.accounts)
	}
}
