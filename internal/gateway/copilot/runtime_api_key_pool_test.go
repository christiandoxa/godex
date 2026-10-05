package copilot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04355CopilotAPIKeyPoolRoutesSyntheticCredentialByAccount(t *testing.T) {
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		auth = append(auth, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"resp_1","output":[]}`)
	}))
	defer server.Close()
	pool, err := NewRuntimeAPIKeyPool(server.URL, []proxymodel.ProviderCredential{
		{ID: "key-a", Secret: "secret-a"},
		{ID: "key-b", Secret: "secret-b"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	response, err := pool.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: copilotMountPath + "/responses", Body: []byte(`{"model":"gpt-6-astra","input":"hi"}`),
	}, proxymodel.Account{ID: "key-b"})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(auth) != 1 || auth[0] != "Bearer secret-b" {
		t.Fatalf("authorization = %#v", auth)
	}
	if !pool.AvailableAccount("key-a") || !pool.AvailableAccount("key-b") || pool.AvailableAccount("missing") {
		t.Fatalf("pool availability mismatch")
	}
}

func TestProdex04355CopilotAPIKeyPoolRejectsIncompleteCredential(t *testing.T) {
	for _, credentials := range [][]proxymodel.ProviderCredential{
		nil, {{ID: "key"}}, {{Secret: "secret"}},
	} {
		if _, err := NewRuntimeAPIKeyPool("https://api.githubcopilot.com", credentials, http.DefaultClient); err == nil ||
			(!strings.Contains(err.Error(), "empty") && !strings.Contains(err.Error(), "incomplete")) {
			t.Fatalf("credentials %#v error = %v", credentials, err)
		}
	}
}
