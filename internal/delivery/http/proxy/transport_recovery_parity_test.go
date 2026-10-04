package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestProxyResponsesRecoversFromPrecommitStreamReadFailure(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "text/event-stream")
		if request.Header.Get("Authorization") == "Bearer token-a" {
			writer.Header().Set("Content-Length", "128")
			_, _ = io.WriteString(writer, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
			return
		}
		_, _ = io.WriteString(writer, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !reflect.DeepEqual(seen, []string{"Bearer token-a", "Bearer token-b"}) || !strings.Contains(string(body), "\"delta\":\"ok\"") {
		t.Fatalf("precommit stream recovery = %d %q after %#v", response.StatusCode, body, seen)
	}
}
