package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex043714QuotaReceiptBoundToActualSuccessfulHTTPIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tokens []proxymodel.Auth
		status int
		valid  bool
	}{
		{"same actual credential", []proxymodel.Auth{{AccessToken: "synthetic-A", AccountID: "owner-1"}, {AccessToken: "synthetic-A", AccountID: "owner-1"}}, 200, true},
		{"credential changed during probe", []proxymodel.Auth{{AccessToken: "synthetic-A", AccountID: "owner-1"}, {AccessToken: "synthetic-B", AccountID: "owner-1"}}, 200, false},
		{"account changed during probe", []proxymodel.Auth{{AccessToken: "synthetic-A", AccountID: "owner-1"}, {AccessToken: "synthetic-A", AccountID: "owner-2"}}, 200, false},
		{"unauthorized receipt", []proxymodel.Auth{{AccessToken: "synthetic-A", AccountID: "owner-1"}}, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-A" || r.Header.Get("ChatGPT-Account-Id") != "owner-1" {
					t.Errorf("wrong authenticated quota request: %#v", r.Header)
				}
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_, _ = io.WriteString(w, `{"rate_limit":{"primary_window":{"used_percent":20,"reset_at":200}}}`)
				}
			}))
			defer server.Close()
			reader := &authReloadSequence{values: tc.tokens}
			quota, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
			if err != nil {
				t.Fatal(err)
			}
			usage, receipt, err := quota.FetchAuthenticatedAtPolicy(t.Context(), "/synthetic", "", false)
			if tc.valid {
				if err != nil || !receipt.Confirmed || receipt.CredentialDigest == ([32]byte{}) || receipt.StartedAt.IsZero() ||
					usage.Primary == nil || usage.Primary.UsedPercent == nil || *usage.Primary.UsedPercent != 20 {
					t.Fatalf("auth receipt=%+v usage=%+v error=%v", receipt, usage, err)
				}
			} else if err == nil || receipt.Confirmed {
				t.Fatalf("untrusted rotated/failed probe accepted: receipt=%+v err=%v", receipt, err)
			}
		})
	}
}
