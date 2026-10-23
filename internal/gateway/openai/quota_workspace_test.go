package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestQuotaWorkspaceNameMatchesAccountShapesAndOrdering(t *testing.T) {
	for _, fixture := range []struct{ name, body, want string }{
		{"list", `{"accounts":[{"id":"other","name":"Other"},{"id":"selected","name":" Team "}]}`, "Team"},
		{"personal", `{"accounts":[{"id":"selected","name":" ","structure":" PERSONAL "}]}`, "Personal"},
		{"map", `{"accounts":{"key":{"account":{"account_id":"selected","name":"Team"}}}}`, "Team"},
		{"ordered excludes", `{"accounts":{"key":{"account":{"account_id":"selected","name":"Team"}},"other":{"account":{"account_id":"other","name":"Other"}}},"account_ordering":["other"]}`, ""},
		{"unknown ordering falls back", `{"accounts":{"key":{"account":{"account_id":"selected","name":"Team"}}},"account_ordering":["missing"]}`, "Team"},
		{"missing map id skips", `{"accounts":{"ignored":{"account":{"name":"Ignored"}},"key":{"account":{"account_id":"selected","name":"Team"}}}}`, "Team"},
		{"missing list id invalidates", `{"accounts":[{"name":"Invalid"},{"id":"selected","name":"Team"}]}`, ""},
		{"invalid ignored map entry", `{"accounts":{"key":{"account":{"account_id":"selected","name":"Team"}},"ignored":{"account":{"name":7}}},"account_ordering":["key"]}`, ""},
		{"null ordering", `{"accounts":[{"id":"selected","name":"Team"}],"account_ordering":null}`, ""},
		{"null ordering entry", `{"accounts":[{"id":"selected","name":"Team"}],"account_ordering":[null]}`, ""},
		{"duplicate known", `{"accounts":[],"accounts":[{"id":"selected","name":"Team"}]}`, ""},
		{"case-sensitive", `{"accounts":[{"ID":"selected","name":"Team"}]}`, ""},
		{"invalid unicode", `{"accounts":[{"id":"selected","name":"\ud800"}]}`, ""},
		{"malformed", `{"accounts":`, ""},
		{"null", `{"accounts":null}`, ""},
		{"duplicate unknown", `{"accounts":[{"id":"selected","name":"Team","unknown":1,"unknown":2}]}`, "Team"},
		{"unicode confusable", `{"accounts":[{"id":"selected","structure":"perſonal"}]}`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := quotaWorkspaceName([]byte(fixture.body), "selected"); got != fixture.want {
				t.Fatalf("name=%q want=%q", got, fixture.want)
			}
		})
	}
}

func TestQuotaWorkspaceLookupKeepsIdentityAndOptionalFailureFallback(t *testing.T) {
	for _, mode := range []string{"success", "failure", "invalid", "oversize", "no-account"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/backend-api/wham/accounts/check" ||
					r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("ChatGPT-Account-Id") != "selected" ||
					r.Header.Get("originator") != "codex_cli_rs" || r.Header.Get("x-openai-codex-luna-reserve") != "" {
					t.Error("workspace request changed its native envelope")
				}
				if mode == "failure" {
					w.WriteHeader(503)
				}
				body := `{"accounts":[{"id":"selected","name":"Team"}]}`
				if mode == "invalid" {
					body = `invalid JSON`
				} else if mode == "oversize" {
					body += strings.Repeat(" ", 8<<20)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			auth := proxymodel.Auth{AccessToken: "synthetic-access", AccountID: "selected"}
			if mode == "no-account" {
				auth.AccountID = ""
			}
			reader := &authReloadSequence{values: []proxymodel.Auth{auth}}
			client, err := NewQuotaClient(server.URL+"/backend-api", nil, reader)
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := client.FetchWorkspaceAt(t.Context(), "/synthetic", "")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "no-account" {
				if workspace != nil || calls != 0 {
					t.Fatal("workspace lookup invented an account")
				}
				return
			}
			want := ""
			if mode == "success" {
				want = "Team"
			}
			if workspace == nil || workspace.ID != "selected" || workspace.Name != want || calls != 1 {
				t.Fatalf("workspace=%+v requests=%d", workspace, calls)
			}
		})
	}
}
