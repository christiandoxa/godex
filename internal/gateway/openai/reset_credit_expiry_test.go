package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type creditRoundTripper func(*http.Request) (*http.Response, error)

func (call creditRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return call(r) }

func TestQuotaReadRetriesTransientFailureButNotTimeout(t *testing.T) {
	for _, fixture := range []struct {
		err      error
		attempts int
	}{{os.ErrDeadlineExceeded, 1}, {errors.New("synthetic connection failure"), 2}} {
		attempts := 0
		transport := creditRoundTripper(func(*http.Request) (*http.Response, error) { attempts++; return nil, fixture.err })
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1/", nil)
		client := &QuotaClient{}
		if _, err := client.doWithClient(&http.Client{Transport: transport}, request); err == nil || attempts != fixture.attempts {
			t.Fatalf("attempts=%d want=%d err=%v", attempts, fixture.attempts, err)
		}
	}
}

const finiteCreditDetails = `{"credits":[{"status":"consumed","expires_at":"2020-01-01T00:00:00Z"},{"status":"available","expires_at":"2026-10-22T19:00:00Z"},{"status":"available","expires_at":"2026-10-23T01:23:45.987+07:00"},{"status":"available","expires_at":null}]}`

func TestCreditExpirySelectsInstantsAndPreservesUnavailable(t *testing.T) {
	for _, fixture := range []struct{ body, want string }{
		{finiteCreditDetails, "1792693425"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23t01:23:45z"}]}`, "1792718625"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23 01:23:45+07:00"}]}`, "1792693425"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T1:23:45+07:00"}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T01:23:45,987+07:00"}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T01:23:45+24:00"}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T01:23:45+01:60"}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"2016-12-31T23:59:60Z"}]}`, "1483228799"},
		{`{"credits":[{"status":"available","expires_at":"0000-01-01T00:00:00Z"}]}`, "-62167219200"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T01:23:45.12345678901Z"}]}`, "1792718625"},
		{`{"credits":[{"status":"available","expires_at":null}]}`, "never"},
		{`{"credits":[]}`, "unavailable"},
		{`{"credits":[{"status":"available"}]}`, "unavailable"},
		{`{"credits":[{"status":"Available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":null},{"status":"available","expires_at":false}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"2026-10-23T01:23:45+07:00"},{"status":"available","expires_at":"invalid"}]}`, "unavailable"},
		{`{"credits":[{"status":7}]}`, "unavailable"},
		{`invalid JSON`, "unavailable"},
		{`[]`, "unavailable"},
		{`{"credits":[null]}`, "unavailable"},
		{`{"credits":[{"status":null,"expires_at":null}]}`, "unavailable"},
		{`{"Credits":[{"status":"available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"Status":"available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"available","Expires_At":null}]}`, "unavailable"},
		{`{"credits":[],"credits":[{"status":"available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"consumed","status":"available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"consumed","sta\u0074us":"available","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":"bad","expires_at":null}]}`, "unavailable"},
		{`{"credits":[{"status":"available","expires_at":null,"unknown":1,"unknown":2}],"ignored":1,"ignored":2}`, "never"},
	} {
		t.Run(fixture.body, func(t *testing.T) {
			credits := &quotamodel.ResetCredits{AvailableCount: 3, NeverExpires: true}
			setResetCreditExpiry(credits, []byte(fixture.body))
			got := "unavailable"
			if credits.NextExpiry != nil {
				got = fmt.Sprint(*credits.NextExpiry)
			} else if credits.NeverExpires {
				got = "never"
			}
			if got != fixture.want || credits.AvailableCount != 3 {
				t.Fatalf("expiry=%s count=%d", got, credits.AvailableCount)
			}
		})
	}
}

func TestCreditDetailsUseAcceptedIdentityAndOnlyDisplayFetches(t *testing.T) {
	for _, mode := range []string{"display", "raw", "runtime", "zero", "failure"} {
		t.Run(mode, func(t *testing.T) {
			paths := make(chan string, 3)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths <- r.URL.Path
				if r.Header.Get("Authorization") != "Bearer synthetic-accepted" || r.Header.Get("ChatGPT-Account-Id") != "synthetic-account" {
					t.Error("detail request reloaded a different credential")
				}
				if r.URL.Path == "/backend-api/wham/usage" {
					count := 3
					if mode == "zero" {
						count = 0
					}
					_, _ = fmt.Fprintf(w, `{"plan_type":"plus","rate_limit_reset_credits":{"available_count":%d}}`, count)
				} else {
					if mode == "failure" {
						w.WriteHeader(500)
					}
					_, _ = w.Write([]byte(finiteCreditDetails))
				}
			}))
			defer server.Close()
			reader := &authReloadSequence{values: []proxymodel.Auth{{AccessToken: "synthetic-accepted", AccountID: "synthetic-account"}, {AccessToken: "synthetic-other", AccountID: "other-account"}}}
			client, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
			if err != nil {
				t.Fatal(err)
			}
			var usage quotamodel.Usage
			switch mode {
			case "raw":
				_, err = client.FetchRaw(t.Context(), "/synthetic")
			case "runtime":
				usage, err = client.Fetch(t.Context(), "/synthetic")
			default:
				usage, err = client.FetchForDisplayAt(t.Context(), "/synthetic", "")
			}
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "display" || mode == "failure" {
				want = 2
			}
			if len(paths) != want {
				t.Fatalf("requests=%d want=%d", len(paths), want)
			}
			if <-paths != "/backend-api/wham/usage" {
				t.Fatal("usage route changed")
			}
			if want == 2 && <-paths != "/backend-api/wham/rate-limit-reset-credits" {
				t.Fatal("detail route changed")
			}
			if mode == "display" && (usage.ResetCredits == nil || usage.ResetCredits.NextExpiry == nil) {
				t.Fatal("display lost known expiry")
			}
			if mode == "failure" && (usage.PlanType != "plus" || usage.ResetCredits.AvailableCount != 3 || usage.ResetCredits.NextExpiry != nil) {
				t.Fatal("optional failure discarded usage")
			}
			if usage.ResetCredits != nil {
				raw, _ := json.Marshal(usage.ResetCredits)
				if string(raw) != fmt.Sprintf(`{"AvailableCount":%d}`, usage.ResetCredits.AvailableCount) {
					t.Fatal("display expiry changed durable JSON")
				}
			}
		})
	}
}
