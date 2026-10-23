package openai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/codex"
)

func TestQuotaAndRedeemHonorProfileResidency(t *testing.T) {
	for _, redeem := range []bool{false, true} {
		for _, fixture := range []struct {
			name, config, want string
			reject             bool
		}{
			{"absent", "", "", false},
			{"us", "enforce_residency='us'", "us", false},
			{"padded uppercase", "enforce_residency=' US '", "us", false},
			{"other", "enforce_residency='eu'", "", false},
			{"unicode confusable", "enforce_residency='uſ'", "", false},
			{"malformed", "enforce_residency=[", "", true},
		} {
			kind := "quota/"
			if redeem {
				kind = "redeem/"
			}
			t.Run(kind+fixture.name, func(t *testing.T) {
				home := writeQuotaAuth(t, "synthetic-token", "synthetic-account")
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(fixture.config), 0o600); err != nil {
					t.Fatal(err)
				}
				observed := make(chan string, 2)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					observed <- r.Header.Get("x-openai-internal-codex-residency")
					_, _ = w.Write([]byte(`{"outcome":"reset","plan_type":"plus"}`))
				}))
				defer upstream.Close()
				client, err := NewQuotaClient(upstream.URL+"/backend-api", nil, codex.NewCodexProcess("", codex.Terminal{}))
				if err != nil {
					t.Fatal(err)
				}
				if redeem {
					_, err = client.ConsumeResetCredit(t.Context(), home, "", false, "synthetic-redeem-id")
				} else {
					_, err = client.Fetch(t.Context(), home)
				}
				if fixture.reject {
					if err == nil || len(observed) != 0 {
						t.Fatalf("invalid profile config reached upstream: err=%v requests=%d", err, len(observed))
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(observed) != 1 {
					t.Fatalf("expected one request, got %d", len(observed))
				}
				if got := <-observed; got != fixture.want {
					t.Errorf("residency=%q want=%q", got, fixture.want)
				}
			})
		}
	}
}
