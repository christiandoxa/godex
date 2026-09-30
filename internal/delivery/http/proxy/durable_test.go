package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestRotatedConversationSurvivesProxyRestart(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	accounts[0].ID = strings.Repeat("a", 32)
	accounts[1].ID = strings.Repeat("b", 32)
	home := t.TempDir()
	bindings := routingrepo.NewStore(home)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if calls == 1 {
			writer.WriteHeader(429)
			return
		}
		if request.Header.Get("ChatGPT-Account-Id") != "workspace-B" {
			t.Error("continuation changed its rotated owner")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"id":"response-%d"}`, calls)
	}))
	defer upstream.Close()
	launch := func() *httptest.Server {
		transport, err := openai.NewTransport(upstream.URL, nil, codex.NewCodexProcess("", codex.Terminal{}))
		if err != nil {
			t.Fatal(err)
		}
		router, err := routingusecase.NewRouter(routingusecase.Config{Gateway: transport, Bindings: bindings, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
		if err != nil {
			t.Fatal(err)
		}
		proxy, err := NewProxy(Config{Router: router})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(proxy)
		t.Cleanup(func() { server.Close(); router.Close() })
		return server
	}
	first := launch()
	response := doProxyJSON(t, first.URL+"/responses", `{}`, map[string]string{"thread-id": "native-thread"})
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	first.Close()
	owner, err := routingusecase.SessionOwner(context.Background(), bindings, "native-thread")
	if err != nil || owner != accounts[1].ID {
		t.Fatalf("durable upstream owner missing: %v", err)
	}
	second := launch()
	response = doProxyJSON(t, second.URL+"/responses", `{"previous_response_id":"response-2"}`, map[string]string{"thread-id": "native-thread"})
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || calls != 3 {
		t.Fatalf("restart status/calls = %d/%d", response.StatusCode, calls)
	}
	snapshot, err := os.ReadFile(filepath.Join(home, "routing.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"native-thread", "response-2", "token-a", "token-b"} {
		if strings.Contains(string(snapshot), raw) {
			t.Fatal("raw continuity or credential data persisted")
		}
	}
}
