package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04357SmartContextExactRoundTripPreservesCriticalSignals(t *testing.T) {
	critical := strings.Repeat(
		"error[E0308]: mismatch src/lib.rs:12:5 @@ -1,2 +1,3 @@ test parses ... FAILED exit code 1 stack backtrace: warning: unused variable\n",
		16,
	)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", critical),
		messageInput("user", critical),
	})
	got := prepareSmartContextHTTPBody(true, "/backend-api/codex/responses", nil, body)
	if !got.Rewritten {
		t.Fatalf("exact inline-reference round trip was rejected: %#v", got)
	}
	if !bytes.Contains(got.Body, []byte("[godex-context-ref ")) {
		t.Fatalf("rewritten body lacks inline reference: %s", got.Body)
	}
	original, ok := smartContextParseJSON(body)
	if !ok {
		t.Fatal("parse original")
	}
	rewritten, ok := smartContextParseJSON(got.Body)
	if !ok {
		t.Fatal("parse rewritten")
	}
	expanded, ok := smartContextExpandInlineReferences(original, rewritten)
	if !ok || !smartContextRoundTripExact(original, expanded) {
		t.Fatal("rewritten critical-signal body is not exact after expansion")
	}
}

func TestProdex04357SmartContextPrepareFallbackEmitsTaggedReasonFields(t *testing.T) {
	recorder := &recordingActivity{}
	gateway := &smartContextCaptureGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{
		Router: router, Activity: recorder, ListenAddr: "127.0.0.1:0", SmartContextEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	body := `{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"short"}]}`
	response := doProxyJSON(t, server.URL+"/backend-api/codex/responses", body, nil)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	event, ok := smartContextEvent(recorder.events, "smart_context_prepare_fallback")
	if !ok {
		t.Fatalf("missing Smart Context fallback event: %#v", recorder.events)
	}
	for key, want := range map[string]string{
		"transport":  "http",
		"route":      "responses",
		"profile":    "-",
		"reason":     "below_minimum_body",
		"decision":   "pass_through",
		"body_bytes": strconv.Itoa(len(body)),
	} {
		if got := event.Fields[key]; got != want {
			t.Fatalf("fallback field %s = %q, want %q; event=%#v", key, got, want, event)
		}
	}
}

func TestProdex04357SmartContextUnsupportedTokenizerEmitsTaggedFallback(t *testing.T) {
	recorder := &recordingActivity{}
	gateway := &smartContextCaptureGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{
		Router: router, Activity: recorder, ListenAddr: "127.0.0.1:0", SmartContextEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	duplicate := strings.Repeat("unsupported tokenizer duplicate ", 80)
	body := string(smartContextFixture("gpt-4", []any{
		messageInput("user", duplicate),
		messageInput("user", duplicate),
	}))
	response := doProxyJSON(t, server.URL+"/backend-api/codex/responses", body, nil)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	event, ok := smartContextEvent(recorder.events, "smart_context_prepare_fallback")
	if !ok || event.Fields["reason"] != "unsupported_tokenizer" {
		t.Fatalf("unsupported-tokenizer fallback = %#v, events=%#v", event, recorder.events)
	}
	if bodies := gateway.snapshot(); len(bodies) != 1 || string(bodies[0]) != body {
		t.Fatalf("unsupported-tokenizer request was changed: %#v", bodies)
	}
}

func smartContextEvent(events []runtimemodel.Event, kind string) (runtimemodel.Event, bool) {
	for _, event := range events {
		if event.Kind == kind {
			return event, true
		}
	}
	return runtimemodel.Event{}, false
}

func TestProdex04357SmartContextValidationReasonPriorityMatchesTaggedMojo(t *testing.T) {
	base := smartContextValidationStats{duplicateTexts: 1}
	cases := []struct {
		name         string
		criticalLoss bool
		fallback     bool
		bits         uint64
		stats        smartContextValidationStats
		want         string
	}{
		{"direct critical signal", true, true, 0, base, "critical_signal_loss"},
		{"reason critical signal", false, true, 1 << 4, base, "critical_signal_loss"},
		{"missing refs before exactness", false, true, (1 << 5) | (1 << 0), base, "missing_rehydrate_refs"},
		{"exactness before empty", false, true, (1 << 0) | (1 << 6), base, "exactness_required"},
		{"empty before token savings", false, true, (1 << 6) | (1 << 3), base, "empty_after_payload"},
		{"token savings before tokenizer", false, true, (1 << 3) | (1 << 1), base, "token_savings_below_safety_margin"},
		{"unsupported tokenizer before budget", false, true, 1 << 1, base, "unsupported_tokenizer"},
		{"token budget default", false, true, 1 << 2, base, "token_budget_did_not_improve"},
		{"no fallback exact", false, false, 1 << 4, base, ""},
		{"rehydrate only", false, true, 1 << 4, smartContextValidationStats{rehydratedRefs: 1}, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := smartContextValidationReason(testCase.criticalLoss, testCase.fallback, testCase.bits, testCase.stats); got != testCase.want {
				t.Fatalf("reason = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestProdex04357SmartContextRewriteEmitsAutopilotStatus(t *testing.T) {
	recorder := &recordingActivity{}
	proxy := &Proxy{activity: recorder}
	duplicate := strings.Repeat("rewrite telemetry duplicate ", 160)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", duplicate),
		messageInput("user", duplicate),
	})
	result := prepareSmartContextHTTPBody(true, "/backend-api/codex/responses", nil, body)
	if !result.Rewritten {
		t.Fatalf("fixture did not rewrite: %#v", result)
	}
	proxy.recordSmartContextResult(t.Context(), 57, "/backend-api/codex/responses", false, len(body), result)
	event, ok := smartContextEvent(recorder.events, "smart_context_autopilot")
	if !ok {
		t.Fatalf("missing autopilot event: %#v", recorder.events)
	}
	for key, want := range map[string]string{
		"transport":       "http",
		"route":           "responses",
		"decision":        "rewritten",
		"self_check":      "ok_saved",
		"rewrite_status":  "ok_saved",
		"fallback_reason": "-",
	} {
		if got := event.Fields[key]; got != want {
			t.Fatalf("autopilot field %s = %q, want %q; event=%#v", key, got, want, event)
		}
	}
}

func TestProdex04357SmartContextWebSocketFallbackEventUsesWebSocketRoute(t *testing.T) {
	recorder := &recordingActivity{}
	proxy := &Proxy{activity: recorder}
	result := smartContextRewrite{Body: []byte(`{"type":"response.create","generate":false}`), FallbackReason: "websocket_generate_false"}
	proxy.recordSmartContextResult(t.Context(), 58, "/backend-api/codex/responses", true, len(result.Body), result)
	event, ok := smartContextEvent(recorder.events, "smart_context_prepare_fallback")
	if !ok {
		t.Fatalf("missing websocket fallback event: %#v", recorder.events)
	}
	if event.Fields["transport"] != "websocket" ||
		event.Fields["route"] != "websocket" ||
		event.Fields["reason"] != "websocket_generate_false" {
		t.Fatalf("websocket fallback event = %#v", event)
	}
}
