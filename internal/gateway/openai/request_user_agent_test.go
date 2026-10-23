package openai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestForwardedUserAgentPreservesClientOwnership(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		for _, fixture := range []struct {
			name    string
			headers http.Header
			want    string
		}{
			{"omitted", nil, ""},
			{"empty", http.Header{"User-Agent": {""}}, ""},
			{"explicit", http.Header{"User-Agent": {"synthetic-codex/0.164.0"}}, "synthetic-codex/0.164.0"},
			{"hop header", http.Header{"Connection": {"User-Agent"}, "User-Agent": {"hop-only"}}, ""},
		} {
			t.Run(fixture.name+map[bool]string{false: "/http", true: "/websocket"}[websocket], func(t *testing.T) {
				observed := make(chan []string, 1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					observed <- r.Header.Values("User-Agent")
					w.WriteHeader(http.StatusOK)
				}))
				defer upstream.Close()
				transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
				if err != nil {
					t.Fatal(err)
				}
				defer transport.Close()
				original := fixture.headers.Clone()
				input := proxymodel.Request{Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: fixture.headers}
				account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
				var response *proxymodel.Response
				if websocket {
					response, err = transport.ExecuteWebSocket(t.Context(), input, account)
				} else {
					response, err = transport.Execute(t.Context(), input, account)
				}
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				var want []string
				if fixture.want != "" {
					want = []string{fixture.want}
				}
				if got := <-observed; !reflect.DeepEqual(got, want) {
					t.Errorf("upstream User-Agent=%q, want %q", got, want)
				}
				if !reflect.DeepEqual(fixture.headers, original) {
					t.Error("forwarding changed client headers")
				}
			})
		}
	}
}
