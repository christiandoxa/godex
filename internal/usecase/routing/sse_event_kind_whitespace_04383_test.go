package routing

import (
	"context"
	"encoding/json"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"io"
	"net/http"
	"testing"
	"time"
)

// The pinned Rust response metadata decoder trims event-type whitespace
// before invoking Mojo's exact-literal, case-sensitive classifier. A padded
// response.created remains a precommit hold; wrong-case remains opaque.
func TestProdex04384SSEEventTypeNormalizationBeforeMojoHold(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	for _, tc := range []struct {
		name, kind string
		wait       bool
	}{
		{"canonical-created", "response.created", true},
		{"canonical-keepalive", "keepalive", true},
		{"canonical-in-progress", "response.in_progress", true},
		{"canonical-reasoning", "response.reasoning_summary_part.added", true},
		{"leading-space", " response.created", true},
		{"trailing-space", "response.created ", true},
		{"trailing-tab", "response.created\t", true},
		{"trailing-newline", "response.created\n", true},
		{"padded-keepalive", "keepalive ", true},
		{"padded-metadata", " response.metadata", true},
		{"wrong-case", "RESPONSE.CREATED", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(`{"type":` + quotedSSEKind04383(tc.kind) + `,"response":{"id":"resp-opaque"}}`)
			outcome, wait := streamOutcome(data, make(http.Header), now, "openai")
			if wait != tc.wait || (!wait && outcome.kind != responsePass) {
				t.Fatalf("event type=%q wait=%t want=%t outcome=%+v", tc.kind, wait, tc.wait, outcome)
			}
		})
	}
}

func quotedSSEKind04383(text string) string { data, _ := json.Marshal(text); return string(data) }

// Rust normalizes first-event type whitespace before Mojo's hold decision.
// A later insufficient_quota remains PRECOMMIT, so the selected profile may
// rotate before releasing any output. The original first response is never
// client-visible. This is verified with complete downstream wire bytes.
func TestProdex04384PaddedSSEMetadataHoldsBeforeLaterQuotaFailure(t *testing.T) {
	for _, tc := range []struct {
		name, firstType string
		wantAttempts    int
		wantOwner       string
	}{
		{"padded-trailing", "response.created ", 2, websocketQuotaOwnerB},
		{"padded-leading", " response.created", 2, websocketQuotaOwnerB},
		{"padded-keepalive", "keepalive ", 2, websocketQuotaOwnerB},
		{"canonical-control", "response.created", 2, websocketQuotaOwnerB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := "data: " + `{"type":` + quotedSSEKind04383(tc.firstType) + `,"response":{"id":"resp-started"}}` + "\n\n" +
				"data: " + `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}` + "\n\n"
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusOK, "text/event-stream", wire},
				{http.StatusOK, "text/event-stream", "data: {\"type\":\"response.completed\"}\n\n"},
			}}
			config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(`{"model":"gpt-6","stream":true,"input":[]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.accounts) != tc.wantAttempts || result.Result.AccountID != tc.wantOwner {
				t.Fatalf("SSE type normalization did not rotate before commitment: attempts=%v owner=%q want=%q", gateway.accounts, result.Result.AccountID, tc.wantOwner)
			}
			body, err := io.ReadAll(result.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			wantClient := "data: {\"type\":\"response.completed\"}\n\n"
			if string(result.Result.Prefix)+string(body) != wantClient {
				t.Fatalf("padded hold leaked failed precommit body: %q", string(result.Result.Prefix)+string(body))
			}
		})
	}
}
