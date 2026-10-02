package deepseek

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestDeepSeekPoolRoutesCredentialByAccountID(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		seen[string(body)] = request.Header.Get("Authorization")
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	pool, err := NewRuntimePool(server.URL, []proxymodel.ProviderCredential{{ID: "key-a", Secret: "secret-a"}, {ID: "key-b", Secret: "secret-b"}}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, fixture := range []struct{ id, body string }{{"key-a", "body-a"}, {"key-b", "body-b"}} {
		response, err := pool.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/chat/completions", Body: []byte(fixture.body)}, proxymodel.Account{ID: fixture.id})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["body-a"] != "Bearer secret-a" || seen["body-b"] != "Bearer secret-b" {
		t.Fatalf("credential routing = %#v", seen)
	}
}
