package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

const heartbeat04379 = "data: {\"type\":\"keepalive\"}\n\n"
const overload04379 = "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\"}}}\n\n"
const visible04379 = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"visible\"}\n\n"

func TestProdex04379PublicResponsesHeartbeatRecoveryAndCommitFence(t *testing.T) {
	for _, tc := range []struct {
		name, mime, first, want string
		owners                  []string
	}{
		{"explicit SSE heartbeat overload rotates", "text/event-stream", heartbeat04379 + overload04379, visible04379, []string{"workspace-A", "workspace-B"}},
		{"headerless requested SSE rotates", "", heartbeat04379 + overload04379, visible04379, []string{"workspace-A", "workspace-B"}},
		{"output after heartbeat forbids rotation", "text/event-stream", heartbeat04379 + visible04379 + overload04379, heartbeat04379 + visible04379 + overload04379, []string{"workspace-A"}},
		{"unknown event still commits", "text/event-stream", "data: {\"type\":\"custom-event\"}\n\n" + overload04379, "data: {\"type\":\"custom-event\"}\n\n" + overload04379, []string{"workspace-A"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
			var mu sync.Mutex
			var owners []string
			const original = `{"stream":true,"model":"synthetic-model","input":[{"role":"user","content":"same-input"}]}`
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil || string(body) != original {
					t.Errorf("upstream body changed: %q error=%v", body, err)
				}
				owner := request.Header.Get("ChatGPT-Account-Id")
				mu.Lock()
				owners = append(owners, owner)
				mu.Unlock()
				if tc.mime == "" {
					writer.Header()["Content-Type"] = nil // suppress Go's content sniffing
				} else {
					writer.Header().Set("Content-Type", tc.mime)
				}
				writer.WriteHeader(http.StatusOK)
				if owner == "workspace-A" {
					_, _ = io.WriteString(writer, tc.first)
					return
				}
				_, _ = io.WriteString(writer, visible04379)
			}))
			defer upstream.Close()
			proxy := newTestProxy(t, upstream.URL, accounts)
			response := doProxyJSON(t, proxy.URL+"/responses", original, nil)
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			mu.Lock()
			got := append([]string(nil), owners...)
			mu.Unlock()
			if err != nil || response.StatusCode != http.StatusOK ||
				string(body) != tc.want || !reflect.DeepEqual(got, tc.owners) {
				t.Fatalf("status=%d body=%q owners=%v error=%v", response.StatusCode, body, got, err)
			}
			if tc.mime == "" && response.Header.Get("Content-Type") != "" {
				t.Fatalf("headerless stream acquired MIME: %q", response.Header.Get("Content-Type"))
			}
		})
	}
}
