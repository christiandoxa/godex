package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04356StartupStreamEventsStayBeforeCommit(t *testing.T) {
	for _, eventType := range []string{
		"codex.rate_limits",
		"codex.response.metadata",
		"response.metadata",
		"response.created",
		"response.in_progress",
		"response.queued",
		"response.output_item.added",
		"response.content_part.added",
		"response.reasoning_summary_part.added",
	} {
		t.Run(eventType, func(t *testing.T) {
			decoder := sse.NewDecoder(1024)
			outcome, done := startupStreamOutcome(
				decoder,
				[]byte("data: {\"type\":\""+eventType+"\"}\n\n"),
				make(http.Header),
				time.Unix(0, 0),
				"openai",
			)
			if done || outcome.kind != responsePass {
				t.Fatalf("startup event committed: done=%t outcome=%#v", done, outcome)
			}
		})
	}

	decoder := sse.NewDecoder(1024)
	if _, done := startupStreamOutcome(
		decoder,
		[]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"),
		make(http.Header),
		time.Unix(0, 0),
		"openai",
	); !done {
		t.Fatal("output event did not commit the stream")
	}
}

func TestProdex04356DeactivatedWorkspaceSSEIsRetryableBeforeCommit(t *testing.T) {
	outcome, wait := streamOutcome(
		[]byte(`{"type":"response.failed","response":{"error":{"code":"deactivated_workspace"}}}`),
		make(http.Header),
		time.Unix(0, 0),
		"openai",
	)
	if wait {
		t.Fatal("deactivated workspace failure was held as startup metadata")
	}
	if outcome.kind != responseRetry || !outcome.profileUnavailable || !outcome.firstEventRetry || !outcome.failed {
		t.Fatalf("deactivated workspace outcome = %#v", outcome)
	}
}

type deactivatedWorkspaceGateway struct {
	mu      sync.Mutex
	calls   []string
	streams map[string]string
}

func (gateway *deactivatedWorkspaceGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.calls = append(gateway.calls, account.ID)
	body := gateway.streams[account.ID]
	gateway.mu.Unlock()
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestProdex04356DeactivatedWorkspaceSSERotatesBeforeCommit(t *testing.T) {
	first := "data: {\"type\":\"response.queued\"}\n\n" +
		"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"deactivated_workspace\"}}}\n\n"
	second := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"response-b\"}}\n\n"
	gateway := &deactivatedWorkspaceGateway{streams: map[string]string{"account-a": first, "account-b": second}}
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
	defer router.Close()

	exchange, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()

	gateway.mu.Lock()
	calls := append([]string(nil), gateway.calls...)
	gateway.mu.Unlock()
	if strings.Join(calls, ",") != "account-a,account-b" {
		t.Fatalf("account attempts = %q", strings.Join(calls, ","))
	}
	if exchange.Result.AccountID != "account-b" || exchange.Result.Failed {
		t.Fatalf("forwarded result = account:%q failed:%t", exchange.Result.AccountID, exchange.Result.Failed)
	}
	if string(exchange.Result.Prefix) != second {
		t.Fatalf("rotated stream prefix = %q, want %q", exchange.Result.Prefix, second)
	}
}
