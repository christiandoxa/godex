package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type admissionOwner04360Gateway struct {
	calls    atomic.Int32
	lastBody string
}

func (g *admissionOwner04360Gateway) Execute(_ context.Context, req proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	g.calls.Add(1)
	g.lastBody = string(req.Body)
	return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"new-response","object":"response"}`))}, nil
}

func admissionOwner04360Setup(t *testing.T) (*activeRequestHandler, *admissionOwner04360Gateway) {
	t.Helper()
	gateway := &admissionOwner04360Gateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Enabled: true, Home: "/fixture/a"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close() })
	if err = router.Observe(t.Context(), "a", make(http.Header), []byte(`{"response":{"id":"owned-response"},"session_id":"owned-session"}`), false); err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, MaxInspectBytes: 4096, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	handler := newActiveRequestHandlerWithLimits(proxy, admissionLimits{global: 3, lane: [admissionLaneCount]int{1, 1, 1, 1}}).(*activeRequestHandler)
	handler.wait = func(context.Context, <-chan struct{}) bool { return false }
	return handler, gateway
}

func TestProdex04360VerifiedContinuationBypassesSaturatedLaneNotGlobalCap(t *testing.T) {
	for _, tc := range []struct {
		name, path, payload string
		lane                admissionLane
	}{
		{"response", "/backend-api/codex/responses", `{"previous_response_id":"owned-response","input":"continuation"}`, admissionLaneResponses},
		{"compact", "/backend-api/codex/responses/compact", `{"session_id":"owned-session","input":"continuation"}`, admissionLaneCompact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, g := admissionOwner04360Setup(t)
			h.active = 1
			h.laneActive[tc.lane] = 1
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.payload))
			writer := httptest.NewRecorder()
			h.ServeHTTP(writer, req)
			if writer.Code != http.StatusOK || g.calls.Load() != 1 || g.lastBody != tc.payload {
				t.Fatalf("owner request was not committed with intact body: code %d, calls %d, body=%q", writer.Code, g.calls.Load(), g.lastBody)
			}
			if h.active != 1 || h.laneActive[tc.lane] != 1 {
				t.Fatalf("lane permit leaked: active=%d lane=%d", h.active, h.laneActive[tc.lane])
			}
		})
	}
}

func TestProdex04360UnverifiedContinuationCannotBypassLaneOrGlobal(t *testing.T) {
	for _, tc := range []struct {
		name, path, payload string
		globalFull          bool
	}{
		{"unowned response", "/backend-api/codex/responses", `{"previous_response_id":"unknown"}`, false},
		{"unowned compact", "/backend-api/codex/responses/compact", `{"session_id":"unknown"}`, false},
		{"owned but global full", "/backend-api/codex/responses", `{"previous_response_id":"owned-response"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, g := admissionOwner04360Setup(t)
			lane := admissionLaneResponses
			if strings.Contains(tc.path, "compact") {
				lane = admissionLaneCompact
			}
			h.active = 1
			if tc.globalFull {
				h.active = h.limits.global
			}
			h.laneActive[lane] = 1
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.payload)))
			if g.calls.Load() != 0 {
				t.Fatalf("unverified or globally saturated request bypassed: %d", g.calls.Load())
			}
		})
	}
}

func TestProdex04360AdmissionPeekIsBoundedAndDoesNotConsumeBody(t *testing.T) {
	h, g := admissionOwner04360Setup(t)
	h.active = 1
	h.laneActive[admissionLaneResponses] = 1
	// A huge input must never be consumed or cause a blind lane bypass.
	body := []byte(`{"previous_response_id":"owned-response","input":"` + strings.Repeat("A", 12000) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses", bytes.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if g.calls.Load() != 0 {
		t.Fatal("oversized, unaudited JSON gained admission priority")
	}
	got, err := io.ReadAll(req.Body)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("bounded peek destroyed request body: %v bytes=%d", err, len(got))
	}
}
