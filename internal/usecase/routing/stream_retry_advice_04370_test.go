package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"net/http"
	"testing"
	"time"
)

func TestProdex04370StructuredSSERetryHeaderWinsOverMessage(t *testing.T) {
	const body = `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 1s.","headers":{"Retry-After":"5"}}}}`
	outcome, wait := streamOutcome([]byte(body), make(http.Header), time.Unix(1000000000, 0), "openai")
	if wait || outcome.kind != responseRetry || !outcome.transient || outcome.quarantine != 5*time.Second {
		t.Fatalf("SSE 0.437.0 structured delay: outcome=%+v wait=%v; want 5s retry", outcome, wait)
	}
}

func TestProdex04370NestedWebSocketRetryHeaderOverridesOuter(t *testing.T) {
	const body = `{"type":"error","status":429,"error":{"code":"rate_limit_exceeded","message":"Please try again in 1s.","headers":{"Retry-After":"5"}},"headers":{"retry-after":"12"}}`
	outcome, wait := streamOutcome([]byte(body), make(http.Header), time.Unix(1000000000, 0), "openai")
	if wait || outcome.kind != responseRetry || outcome.quarantine != 5*time.Second {
		t.Fatalf("WebSocket JSON precedence: outcome=%+v wait=%v; want 5s", outcome, wait)
	}
}

func TestProdex04370StreamHeaderInvalidFallsBackToMessage(t *testing.T) {
	for _, tc := range []struct {
		name, headers string
		want          time.Duration
	}{
		{"control", `{"Retry-After":"\n5\n"}`, 12 * time.Second},
		{"duplicate_last", `{"Retry-After":"30","retry-after":"5"}`, 5 * time.Second},
		{"duplicate_bad_last", `{"Retry-After":"30","retry-after":"invalid"}`, 12 * time.Second},
		{"zero", `{"Retry-After":"0"}`, 0},
		{"invalid", `{"Retry-After":"oops"}`, 12 * time.Second},
		{"huge", `{"Retry-After":"99999"}`, 300 * time.Second},
		{"tab", `{"Retry-After":"\t5\t"}`, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Please try again in 12s.","headers":` + tc.headers + `}}}`
			outcome, _ := streamOutcome([]byte(event), make(http.Header), time.Unix(1000000000, 0), "openai")
			if outcome.kind != responseRetry || outcome.quarantine != tc.want {
				t.Fatalf("header %s: got=%s want=%s", tc.headers, outcome.quarantine, tc.want)
			}
		})
	}
}

func TestProdex04370StreamQuotaAndSuccessIgnoreRetryAdvice(t *testing.T) {
	for _, fixture := range []struct {
		event string
		want  responseKind
	}{
		{`{"type":"response.failed","response":{"error":{"code":"insufficient_quota","headers":{"Retry-After":"5"}}}}`, responseRetry},
		{`{"type":"response.completed","headers":{"Retry-After":"5"}}`, responsePass},
	} {
		outcome, wait := streamOutcome([]byte(fixture.event), make(http.Header), time.Unix(1000000000, 0), "openai")
		if wait || outcome.kind != fixture.want {
			t.Fatalf("quota/committed class changed: %+v wait:%v", outcome, wait)
		}
		if outcome.quarantine == 5*time.Second {
			t.Fatal("structured retry advice reclassified quota/success")
		}
	}
}

// WebSocket transport forwards only a bounded precommit JSON header source;
// user-visible frames are still governed by the existing no-replay fence.
func TestProdex04370WebSocketPrecommitCarriesExactRetryAdvice(t *testing.T) {
	const event = `{"type":"error","status":429,"error":{"code":"rate_limit_exceeded","message":"Please try again in 1s.","headers":{"Retry-After":"5"}},"headers":{"Retry-After":"12"}}`
	router := &Router{now: time.Now}
	for _, fixture := range []struct {
		name        string
		committed   bool
		expectRetry bool
	}{
		{"precommit", false, true},
		{"postcommit", true, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode:          http.StatusOK,
				FirstEventCommitted: fixture.committed,
				PrecommitFailure: &proxymodel.PrecommitFailure{
					Code:            "rate_limit_exceeded",
					RetryAdviceJSON: []byte(event),
				},
			}
			outcome, _, err := router.classifyPrecommitFailure(response, &pendingResponse{response: response})
			if err != nil {
				t.Fatal(err)
			}
			if fixture.expectRetry {
				if outcome.kind != responseRetry || outcome.quarantine != 5*time.Second || !outcome.explicitRetryAdvice {
					t.Fatalf("structured websocket retry was lost: %+v", outcome)
				}
			} else if outcome.kind == responseRetry {
				t.Fatalf("post-commit WebSocket frame was replayed: %+v", outcome)
			}
		})
	}
}

func TestProdex04370ExplicitZeroRetryDelayDoesNotCreateCooldown(t *testing.T) {
	router := &Router{now: time.Now, quarantine: make(map[string]quarantineState), quotaBlocked: make(map[string]bool)}
	outcome := responseOutcome{kind: responseRetry, quarantine: 0, explicitRetryAdvice: true, transient: true}
	router.applyRetryOutcome(t.Context(), "synthetic-primary", quotamodel.Selection{}, outcome)
	if remaining := router.quarantineRemaining("synthetic-primary", router.now()); remaining != 0 {
		t.Fatalf("structured zero Retry-After became backoff: %v", remaining)
	}
}
