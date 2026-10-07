package routing

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04357SelectionPickRecordsQuotaRemainingFromRankingCache(t *testing.T) {
	now := time.Unix(10_000, 0)
	recorder := &routingMarkerRecorder{}
	gateway := &pressureGateway{}
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"main": {
			Ready: true,
			Pressure: quotamodel.Pressure{
				Known: true, FiveHourRemaining: 73, WeeklyRemaining: 61,
				FiveHourResetAt: now.Add(time.Hour).Unix(), WeeklyResetAt: now.Add(24 * time.Hour).Unix(),
			},
			Source: quotamodel.SourceLive,
		},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, Activity: recorder, QuotaPreflight: quota, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Home: "/main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, exchange.Result.Response.Body)
	exchange.Close()

	var selectionFound bool
	var inflight []string
	for _, event := range recorder.events {
		switch event.Kind {
		case "selection_pick":
			selectionFound = true
			if event.AccountID != "main" || event.Fields["profile"] != "main" ||
				event.Fields["five_hour_remaining"] != "73" ||
				event.Fields["weekly_remaining"] != "61" ||
				event.Fields["route"] != "responses" {
				t.Fatalf("selection event = %#v", event)
			}
		case "profile_inflight":
			inflight = append(inflight, event.Fields["count"])
		}
	}
	if !selectionFound {
		t.Fatalf("selection_pick missing: %#v", recorder.events)
	}
	if len(inflight) < 2 || inflight[0] != "2" || inflight[len(inflight)-1] != "0" {
		t.Fatalf("profile_inflight counts = %#v events=%#v", inflight, recorder.events)
	}
}

func TestProdex04357SelectionPickOmitsRunwayFieldsWhenQuotaPressureUnknown(t *testing.T) {
	now := time.Unix(10_100, 0)
	recorder := &routingMarkerRecorder{}
	gateway := &pressureGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, Activity: recorder, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Home: "/main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange.Close()
	for _, event := range recorder.events {
		if event.Kind != "selection_pick" {
			continue
		}
		if _, ok := event.Fields["five_hour_remaining"]; ok {
			t.Fatalf("unknown pressure leaked five-hour runway field: %#v", event)
		}
		if _, ok := event.Fields["weekly_remaining"]; ok {
			t.Fatalf("unknown pressure leaked weekly runway field: %#v", event)
		}
		return
	}
	t.Fatalf("selection_pick missing: %#v", recorder.events)
}
