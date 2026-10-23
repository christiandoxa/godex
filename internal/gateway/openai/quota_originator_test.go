package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/codex"
)

func TestQuotaRequestsHonorCodexOriginator(t *testing.T) {
	for _, fixture := range []struct{ name, value, want string }{
		{"custom", "codex-tui", "codex-tui"},
		{"empty", "", ""},
		{"invalid", "invalid\noriginator", "codex_cli_rs"},
	} {
		for _, operation := range []string{"usage", "display", "redeem"} {
			t.Run(fixture.name+"/"+operation, func(t *testing.T) {
				t.Setenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", fixture.value)
				home := writeQuotaAuth(t, "synthetic-token", "synthetic-account")
				calls := make(chan string, 3)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls <- r.Header.Get("originator")
					if _, present := r.Header["User-Agent"]; present {
						t.Error("quota request invented a User-Agent without native metadata")
					}
					if r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("ChatGPT-Account-Id") != "synthetic-account" {
						t.Error("originator changed credential ownership")
					}
					_, _ = w.Write([]byte(`{"plan_type":"plus","rate_limit_reset_credits":{"available_count":1},"credits":[{"status":"available","expires_at":null}],"outcome":"reset"}`))
				}))
				defer server.Close()
				client, err := NewQuotaClient(server.URL+"/backend-api", nil, codex.NewCodexProcess("missing-codex-header-fixture", codex.Terminal{}))
				if err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "display":
					_, err = client.FetchForDisplayAt(t.Context(), home, "")
				case "redeem":
					_, err = client.ConsumeResetCredit(t.Context(), home, "", false, "synthetic-request-id")
				default:
					_, err = client.Fetch(t.Context(), home)
				}
				if err != nil {
					t.Fatal(err)
				}
				wantCalls := 1
				if operation == "display" {
					wantCalls = 2
				}
				if len(calls) != wantCalls {
					t.Fatalf("requests=%d want=%d", len(calls), wantCalls)
				}
				for range wantCalls {
					if got := <-calls; got != fixture.want {
						t.Errorf("originator=%q want=%q", got, fixture.want)
					}
				}
			})
		}
	}
}
