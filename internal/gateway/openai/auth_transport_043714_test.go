package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The response to an HTTP 401 must carry the identity of the credentials
// actually sent, including after an unauthorized-auth refresh retry. A later
// auth.json read would not be authoritative for the failed request.
func TestProdex043714FailedUpstreamResponseCarriesRequestCredentialDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-A" {
			t.Errorf("wrong request token")
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"unauthorized"}}`)
	}))
	defer server.Close()
	reader := &authReloadSequence{values: []proxymodel.Auth{{AccessToken: "synthetic-A", AccountID: "owner-1"}}}
	client, err := NewTransport(server.URL, nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	response, err := client.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/responses"}, proxymodel.Account{ID: "profile-a", Home: "/synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	digest, valid := openAIAuthDigest043714(proxymodel.Auth{AccessToken: "synthetic-A", AccountID: "owner-1"})
	if !valid || response.AuthCredentialDigest != digest || !response.AuthCredentialKnown || response.AuthRequestStartedAt.IsZero() {
		t.Fatalf("HTTP 401 lost causal request identity: %+v", response)
	}
}
