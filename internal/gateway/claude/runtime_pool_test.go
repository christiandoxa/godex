package claude

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestAnthropicRuntimePoolUsesCredentialFromSelectedAccount(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		seen[string(body)] = request.Header.Get("Authorization")
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte("{\"ok\":true}"))
	}))
	defer server.Close()

	accounts := make([]proxymodel.Account, 0, 3)
	for _, fixture := range []struct {
		id, token, body string
	}{
		{"account-a", "token-a", "body-a"},
		{"account-b", "token-b", "body-b"},
	} {
		home := t.TempDir()
		content := "{\"accessToken\":\"" + fixture.token + "\",\"expiresAt\":4102444800000}"
		if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, proxymodel.Account{
			ID: fixture.id, Home: home, Enabled: true,
			Provider: proxymodel.Provider{Kind: "anthropic", APIURL: server.URL + "/v1"},
		})
	}
	accounts = append(accounts, proxymodel.Account{
		ID: "broken", Home: t.TempDir(), Enabled: true,
		Provider: proxymodel.Provider{Kind: "anthropic", APIURL: server.URL + "/v1"},
	})

	pool, err := NewSource().NewRuntimePool(context.Background(), accounts)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if pool.AvailableAccount("broken") {
		t.Fatal("broken Anthropic profile entered runtime pool")
	}
	for index, account := range accounts[:2] {
		body := []byte("body-a")
		if index == 1 {
			body = []byte("body-b")
		}
		response, err := pool.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: anthropicMountPath + "/chat/completions", Body: body,
		}, account)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["body-a"] != "Bearer token-a" || seen["body-b"] != "Bearer token-b" {
		t.Fatalf("credential routing = %#v", seen)
	}
}
