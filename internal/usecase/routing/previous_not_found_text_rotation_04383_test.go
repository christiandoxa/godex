package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04383PlainPreviousResponseTextFailsClosedForBoundOwner(t *testing.T) {
	for _, tc := range []struct {
		name, message      string
		retry, fullHistory bool
	}{
		{"text-code", "previous_response_not_found", true, false},
		{"text-code-full-history", "previous_response_not_found", true, true},
		{"text-human-full-history", "previous response chain not found", true, true},
		{"text-human", "previous response chain not found", true, false},
		{"text-invalid", "Invalid `previous_response_id`.", true, false},
		{"text-tool", "invalid_request_error: no tool call found for call_id", true, false},
		{"text-unrelated", "unrelated retryable server response", false, false},
		{"malformed-unicode-code", `{"error":{"code":"previous_response_not_found"},"unused":"\ud800"}`, false, false},
		{"malformed-overflow-code", `{"error":{"code":"previous_response_not_found"},"unused":1e309}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusBadRequest, "text/plain", tc.message},
				{http.StatusOK, "application/json", `{"id":"response-fallback","object":"response"}`},
			}}
			accounts := []proxymodel.Account{
				{ID: "account-a", Home: "/synthetic/a", Enabled: true},
				{ID: "account-b", Home: "/synthetic/b", Enabled: true},
			}
			router, err := NewRouter(Config{Gateway: gateway, PreferredAccount: "account-a", Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-owned"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"model":"gpt-6","previous_response_id":"resp-owned","input":[{"role":"user","content":"continue"}]}`
			if tc.fullHistory {
				body = `{"model":"gpt-6","session_id":"safe-session","previous_response_id":"resp-owned","input":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"continue"}]}`
			}
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(body),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			response, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			expectedAttempts, expectedOwner, expectedStatus := 1, "account-a", http.StatusBadRequest
			if tc.retry {
				expectedStatus = http.StatusConflict
			}
			if len(gateway.requests) != expectedAttempts || response.Result.AccountID != expectedOwner || response.Result.Response.StatusCode != expectedStatus {
				t.Fatalf("text=%s selected=%q status=%d attempts=%v; want owner=%q status=%d attempts=%d", tc.name, response.Result.AccountID, response.Result.Response.StatusCode, gateway.accounts, expectedOwner, expectedStatus, expectedAttempts)
			}
			for _, sent := range gateway.requests {
				if string(sent.Body) != string(request.Body) {
					t.Fatal("plain text response replays must preserve opaque request")
				}
			}
			payload, err := io.ReadAll(response.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tc.retry && !strings.Contains(string(payload), `"stale_continuation"`) {
				t.Fatalf("recognized previous error did not fail closed: %s", payload)
			}
			if !tc.retry && !strings.Contains(string(response.Result.Prefix)+string(payload), tc.message) {
				t.Fatalf("unrelated error was rewritten: %q", payload)
			}
		})
	}
}
