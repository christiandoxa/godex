package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type routeDecisionGateway struct{}

func (routeDecisionGateway) Execute(_ context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp-trace"}`)),
	}, nil
}

func TestProdex04357RouteDecisionTraceFreshSelectionMatchesTaggedCompactSchema(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{
		Activity: recorder,
		Gateway:  routeDecisionGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "alpha", Home: "/synthetic/alpha", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		RequestID: 77,
		Path:      "/backend-api/codex/responses",
		Header:    make(http.Header),
		Body:      []byte(`{"model":" gpt-5.6 "}`),
		QuotaSelection: quotamodel.Selection{
			RouteKind:      quotamodel.RouteKindResponses,
			RequestedModel: " gpt-5.6 ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()

	event, ok := routeDecisionEvent(recorder.events)
	if !ok {
		t.Fatalf("missing route_decision event: %#v", recorder.events)
	}
	if event.RequestID != "77" ||
		event.Fields["schema_version"] != "1" ||
		event.Fields["route"] != "responses" ||
		event.Fields["outcome"] != "selected" {
		t.Fatalf("route_decision fields = %#v", event)
	}
	var trace map[string]any
	if err := json.Unmarshal([]byte(event.Fields["trace"]), &trace); err != nil {
		t.Fatalf("trace JSON = %q: %v", event.Fields["trace"], err)
	}
	want := map[string]any{
		"schema_version":     float64(1),
		"route":              "responses",
		"requested_model":    "gpt-5.6",
		"resolved_model":     "gpt-5.6",
		"selected_candidate": "alpha",
		"terminal_outcome":   "selected",
	}
	for key, expected := range want {
		if trace[key] != expected {
			t.Fatalf("trace[%s] = %#v, want %#v; trace=%#v", key, trace[key], expected, trace)
		}
	}
	if _, exists := trace["terminal_reason"]; exists {
		t.Fatalf("selected trace unexpectedly contains terminal_reason: %#v", trace)
	}
	if _, exists := trace["candidates"]; exists {
		t.Fatalf("compact tagged trace unexpectedly contains candidates: %#v", trace)
	}
}

func TestProdex04357RouteDecisionTraceBoundSelectionAndNoCandidate(t *testing.T) {
	t.Run("bound", func(t *testing.T) {
		recorder := &routingMarkerRecorder{}
		now := time.Unix(123, 0)
		router, err := NewRouter(Config{
			Activity: recorder,
			Gateway:  routeDecisionGateway{},
			Now:      func() time.Time { return now },
			Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: "beta", Home: "/synthetic/beta", Enabled: true}}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := router.affinity.remember(t.Context(), "beta", affinityKeys{session: "session-trace"}, now); err != nil {
			t.Fatal(err)
		}
		exchange, err := router.Forward(t.Context(), proxymodel.Request{
			RequestID:      78,
			Path:           "/backend-api/codex/responses",
			Header:         make(http.Header),
			Body:           []byte(`{"client_metadata":{"session_id":"session-trace"}}`),
			QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer exchange.Close()
		event, ok := routeDecisionEvent(recorder.events)
		if !ok || !strings.Contains(event.Fields["trace"], `"selected_candidate":"beta"`) {
			t.Fatalf("bound route decision = %#v, events=%#v", event, recorder.events)
		}
	})

	t.Run("no candidate", func(t *testing.T) {
		recorder := &routingMarkerRecorder{}
		router, err := NewRouter(Config{
			Activity: recorder,
			Gateway:  routeDecisionGateway{},
			Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: "disabled", Home: "/synthetic/disabled", Enabled: false}}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = router.Forward(t.Context(), proxymodel.Request{
			RequestID:      79,
			Path:           "/backend-api/codex/responses/compact",
			Header:         make(http.Header),
			QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindCompact},
		})
		if err == nil {
			t.Fatal("no-candidate request unexpectedly succeeded")
		}
		event, ok := routeDecisionEvent(recorder.events)
		if !ok ||
			event.Fields["route"] != "responses_compact" ||
			event.Fields["outcome"] != "no_candidate" ||
			!strings.Contains(event.Fields["trace"], `"terminal_outcome":"no_candidate"`) {
			t.Fatalf("no-candidate route decision = %#v, events=%#v", event, recorder.events)
		}
	})
}

func routeDecisionEvent(events []runtimemodel.Event) (runtimemodel.Event, bool) {
	for _, event := range events {
		if event.Kind == "route_decision" {
			return event, true
		}
	}
	return runtimemodel.Event{}, false
}

func TestProdex04357RouteDecisionSafeIdentifierTrimsAndBoundsUTF8(t *testing.T) {
	if got, truncated := routeDecisionSafeIdentifier("  model-x  "); got != "model-x" || truncated {
		t.Fatalf("trimmed identifier = %q/%t", got, truncated)
	}
	long := strings.Repeat("a", 95) + "é"
	got, truncated := routeDecisionSafeIdentifier(long)
	if got != strings.Repeat("a", 95) || !truncated || len(got) > 96 {
		t.Fatalf("UTF-8 bounded identifier = %q len=%d truncated=%t", got, len(got), truncated)
	}
}
