package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type prodex04356CompactTurnGateway struct {
	calls []string
}

func (gateway *prodex04356CompactTurnGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	if account.ID == "account-a" {
		return nil, errors.New("connection reset by peer")
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"id":"compact-ok"}`)),
	}, nil
}

func TestProdex04356UnboundTurnStateCompactTransportRotatesToReadyProfile(t *testing.T) {
	gateway := &prodex04356CompactTurnGateway{}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "account-a",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method:         http.MethodPost,
		Header:         http.Header{"X-Codex-Turn-State": []string{"turn-unbound"}},
		Body:           []byte(`{"input":[{"role":"user","content":"compact"}]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		exchange.Result.AccountID != "account-b" ||
		exchange.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("calls/owner/status = %s/%s/%d, want account-a,account-b/account-b/200",
			got, exchange.Result.AccountID, exchange.Result.Response.StatusCode)
	}
	owner, ownerErr := router.affinity.owner(
		t.Context(), affinityKeys{turn: "turn-unbound"}, router.now(),
	)
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if owner != "account-b" {
		t.Fatalf("committed turn-state owner = %q, want account-b", owner)
	}
}
