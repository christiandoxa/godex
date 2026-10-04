package routing

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type prodex04354RecoveryGateway struct {
	calls []string
}

func (gateway *prodex04354RecoveryGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func TestProdex04354FreshRecoveryReloadsCandidatesAfterWait(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}
	loads := 0
	var waits []time.Duration
	gateway := &prodex04354RecoveryGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			loads++
			return append([]proxymodel.Account(nil), accounts...), nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			accounts = []proxymodel.Account{{ID: "account-b", Home: "/b", Enabled: true}}
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", time.Second)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-b" ||
		len(gateway.calls) != 1 || gateway.calls[0] != "account-b" ||
		loads < 2 || len(waits) != 1 {
		t.Fatalf("owner/calls/loads/waits = %q/%v/%d/%v, want account-b/[account-b]/>=2/[1s]",
			exchange.Result.AccountID, gateway.calls, loads, waits)
	}
}

func TestProdex04354RecoveryReselectionDoesNotConsumeNextRequestRoundRobinSlot(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &prodex04354CrossRequestGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 21})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 22})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	want := []string{"account-a", "account-b", "account-a", "account-b"}
	if first.Result.AccountID != "account-a" || second.Result.AccountID != "account-b" ||
		!reflect.DeepEqual(gateway.calls, want) || len(waits) != 1 {
		t.Fatalf("owners/calls/waits = %q,%q/%v/%v, want account-a,account-b/%v/1 wait",
			first.Result.AccountID, second.Result.AccountID, gateway.calls, waits, want)
	}
}

type prodex04354CrossRequestGateway struct {
	calls []string
}

func (gateway *prodex04354CrossRequestGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	status := http.StatusOK
	if len(gateway.calls) <= 2 {
		status = http.StatusServiceUnavailable
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func TestProdex04354RecoveryWaitDoesNotAdvanceRequestRotation(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &prodex04354SweepGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 17})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	want := []string{"account-a", "account-b", "account-a", "account-b", "account-a"}
	if exchange.Result.AccountID != "account-a" || !reflect.DeepEqual(gateway.calls, want) || len(waits) != 2 {
		t.Fatalf("owner/calls/waits = %q/%v/%v, want account-a/%v/2 waits",
			exchange.Result.AccountID, gateway.calls, waits, want)
	}
}

type prodex04354SweepGateway struct {
	calls []string
}

func (gateway *prodex04354SweepGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	status := http.StatusServiceUnavailable
	body := ""
	if len(gateway.calls) == 5 {
		status = http.StatusOK
		body = "{}"
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestProdex04354FreshRecoveryDoesNotRequireTransientFailureFlag(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &prodex04354RecoveryGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", time.Second)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" ||
		len(gateway.calls) != 1 || gateway.calls[0] != "account-a" ||
		len(waits) != 1 {
		t.Fatalf("owner/calls/waits = %q/%v/%v, want account-a/[account-a]/[1s]",
			exchange.Result.AccountID, gateway.calls, waits)
	}
}
