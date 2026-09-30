package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const quotaEvent = "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"usage_limit_reached\"}}}\n\n"

func TestProxySSEQuotaRotatesOnlyBeforeOutput(t *testing.T) {
	metadata := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"discarded\"}}\n\n"
	output := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"text\"}\n\n"
	for _, test := range []struct {
		name, prefix string
		rotate       bool
	}{
		{"initial quota", "", true}, {"startup metadata", metadata, true},
		{"after output", metadata + output, false}, {"unknown event", "data: {}\n\n", false},
		{"inspection ceiling", ":" + strings.Repeat("x", 64<<10) + "\n\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
			var seen []string
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				seen = append(seen, request.Header.Get("ChatGPT-Account-Id"))
				writer.Header().Set("Content-Type", "text/event-stream")
				if len(seen) == 1 {
					io.WriteString(writer, test.prefix+quotaEvent)
					return
				}
				io.WriteString(writer, output)
			}))
			defer upstream.Close()
			proxy := newTestProxy(t, upstream.URL, accounts)
			response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			want, owners := test.prefix+quotaEvent, []string{"workspace-A"}
			if test.rotate {
				want, owners = output, []string{"workspace-A", "workspace-B"}
			}
			if err != nil || response.StatusCode != 200 || string(body) != want || !reflect.DeepEqual(seen, owners) {
				t.Fatalf("SSE bytes or retry policy changed: status=%d, owners=%v, error=%v", response.StatusCode, seen, err)
			}
		})
	}
}

func TestFailedStartupStreamDoesNotClaimConversation(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	available := accounts[:1]
	var seen []string
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		seen = append(seen, request.Header.Get("ChatGPT-Account-Id"))
		body := quotaEvent
		if len(seen) > 1 {
			body = "data: {\"type\":\"response.completed\"}\n\n"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	proxy, err := newProxyForTest(ProxyConfig{Client: &http.Client{Transport: transport}, Accounts: func(context.Context) ([]RuntimeAccount, error) { return available, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.router.Close()
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{}`))
		request.Header.Set("thread-id", "failed-startup")
		writer := httptest.NewRecorder()
		proxy.ServeHTTP(writer, request)
		if writer.Code != 200 || (attempt == 0 && writer.Body.String() != quotaEvent) {
			t.Fatalf("failed startup fixed conversation ownership: %d", writer.Code)
		}
		accounts[0].Enabled = false
		available = accounts
	}
	if !reflect.DeepEqual(seen, []string{"workspace-A", "workspace-B"}) {
		t.Fatalf("startup failure replayed or claimed owner: %v", seen)
	}
}

func TestLateMultilineSSEOwnershipSurvivesRestart(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	accounts[0].ID, accounts[1].ID = strings.Repeat("a", 32), strings.Repeat("b", 32)
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"text\"}\n\n"
	stream += "data: {\"delta\":\"" + strings.Repeat("x", 70<<10) + "\"}\n\n"
	stream += "event: response.completed\r\ndata: {\"type\":\"response.completed\",\r\ndata: \"response\":{\"id\":\"late-response\",\"turn_state\":\"late-state\"}}\r\n\r\n"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Header.Get("ChatGPT-Account-Id") != "workspace-A" {
			t.Error("late continuation changed account")
		}
		if calls == 1 {
			writer.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(writer, stream)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{}`)
	}))
	defer upstream.Close()
	bindings := routingrepo.NewStore(t.TempDir())
	launch := func(preferred string) *httptest.Server {
		proxy, err := newProxyForTest(ProxyConfig{UpstreamURL: upstream.URL, Bindings: bindings, PreferredAccount: preferred, Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil }})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(proxy)
		t.Cleanup(func() { server.Close(); proxy.router.Close() })
		return server
	}
	first := launch(accounts[0].ID)
	response := doProxyJSON(t, first.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	first.Close()
	if err != nil || string(body) != stream {
		t.Fatalf("long stream framing changed: %v", err)
	}
	second := launch(accounts[1].ID)
	response = doProxyJSON(t, second.URL+"/responses", `{"previous_response_id":"late-response"}`, map[string]string{"x-codex-turn-state": "late-state"})
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || calls != 2 {
		t.Fatalf("late metadata lost after restart: status=%d, calls=%d", response.StatusCode, calls)
	}
}
