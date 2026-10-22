package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestGeminiRuntimeSeparatesLocalAndGatewayConversationScopes(t *testing.T) {
	for _, fixture := range []struct {
		name, seedSession, compactSession, account string
		gateway, replay                            bool
	}{
		{name: "local same session", seedSession: "session-a", compactSession: "session-a", replay: true},
		{name: "local changed session", seedSession: "session-a", compactSession: "session-b", replay: true},
		{name: "local omitted session", seedSession: "session-a", replay: true},
		{name: "local added session", compactSession: "session-b", replay: true},
		{name: "gateway same session", seedSession: "session-a", compactSession: "session-a", gateway: true, replay: true},
		{name: "gateway different session", seedSession: "session-a", compactSession: "session-b", gateway: true},
		{name: "gateway omitted session", seedSession: "session-a", gateway: true},
		{name: "local different credential", seedSession: "session-a", compactSession: "session-b", account: "secondary"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				requests <- request
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"upstream-id","responseId":"resp-synthetic","candidates":[{"content":{"parts":[{"text":"synthetic-ok"}]}}]}`)
			}))
			defer server.Close()
			gateway, err := newRuntimeGateway(proxymodel.Config{
				GatewayMode: fixture.gateway, Provider: proxymodel.Provider{Kind: "gemini", APIURL: server.URL},
				ProviderCredentials: []proxymodel.ProviderCredential{{ID: "primary", Secret: "synthetic-primary"}, {ID: "secondary", Secret: "synthetic-secondary"}},
			}, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer gateway.(interface{ Close() }).Close()
			for index, session := range []string{fixture.seedSession, fixture.compactSession} {
				request := map[string]any{"input": "synthetic user"}
				path := "/backend-api/prodex/responses"
				account := "primary"
				if index == 1 {
					request["input"] = []any{}
					request["previous_response_id"] = "resp-synthetic"
					path += "/compact"
					if fixture.account != "" {
						account = fixture.account
					}
				}
				if session != "" {
					request["session_id"] = session
				}
				body, _ := json.Marshal(request)
				response, err := gateway.Execute(t.Context(), proxymodel.Request{Method: "POST", Path: path, Body: body}, proxymodel.Account{ID: account})
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatalf("response status=%d", response.StatusCode)
				}
			}
			if len(requests) != 2 {
				t.Fatal("scope changed attempt count")
			}
			<-requests
			compact := <-requests
			messages, _ := compact["messages"].([]any)
			want := 2
			if fixture.replay {
				want = 3
			}
			if len(messages) != want {
				t.Fatalf("scope replay messages=%#v, want %d", messages, want)
			}
			if fixture.replay && messages[1].(map[string]any)["content"] != "synthetic user" {
				t.Fatal("cached user message lost")
			}
		})
	}
}
