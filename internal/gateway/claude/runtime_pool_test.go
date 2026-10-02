package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
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

func TestAnthropicAPIKeyPoolExhaustsModelsBeforeCredentialRotation(t *testing.T) {
	var mu sync.Mutex
	attempts := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		model, _ := value["model"].(string)
		auth := request.Header.Get("Authorization")
		mu.Lock()
		attempts = append(attempts, auth+":"+model)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if auth == "Bearer key-one" {
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(`{"error":{"code":"quota_exhausted"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"claude-sonnet-5-5","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	credentials := []proxymodel.ProviderCredential{{ID: "key-1", Secret: "key-one"}, {ID: "key-2", Secret: "key-two"}}
	pool, err := NewRuntimeAPIKeyPool(server.URL+"/v1", credentials, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	accounts := []proxymodel.Account{
		{ID: "key-1", Home: "/synthetic/key-home", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
		{ID: "key-2", Home: "/synthetic/key-home", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: pool, PreferredAccount: "key-1",
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/responses",
		Body: []byte(`{"model":"sonnet","input":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "key-2" || exchange.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("result = %#v", exchange.Result)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"Bearer key-one:claude-sonnet-5-5",
		"Bearer key-one:claude-opus-5-5",
		"Bearer key-two:claude-sonnet-5-5",
	}
	if strings.Join(attempts, "|") != strings.Join(want, "|") {
		t.Fatalf("attempts = %#v, want %#v", attempts, want)
	}
}

func TestAnthropicAPIKeyPoolBare429DoesNotRotateCredentialOrModel(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"too many requests"}}`))
	}))
	defer server.Close()
	pool, err := NewRuntimeAPIKeyPool(server.URL+"/v1", []proxymodel.ProviderCredential{
		{ID: "key-1", Secret: "key-one"}, {ID: "key-2", Secret: "key-two"},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	accounts := []proxymodel.Account{
		{ID: "key-1", Home: "/synthetic/key-home", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
		{ID: "key-2", Home: "/synthetic/key-home", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: pool, PreferredAccount: "key-1",
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/responses",
		Body: []byte(`{"model":"sonnet","input":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if calls != 1 || exchange.Result.AccountID != "key-1" || exchange.Result.Response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("calls/result = %d / %#v", calls, exchange.Result)
	}
}
