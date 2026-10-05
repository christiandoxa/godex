package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyResponsesRecoversFromPrecommitStreamReadFailure(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(request.Header.Get("Authorization"), "token-a") {
			writer.Header().Set("Content-Length", "128")
			_, _ = io.WriteString(writer, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
			return
		}
		_, _ = io.WriteString(writer, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
	}))
	defer upstream.Close()

	proxy := newTestProxy(t, upstream.URL, accounts)
	for attempt := 0; attempt < 3; attempt++ {
		response := doProxyJSON(t, proxy.URL+"/responses", `{}`, nil)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "\"delta\":\"ok\"") {
			t.Fatalf("precommit stream recovery attempt %d = %d %q after %#v", attempt, response.StatusCode, body, seen)
		}
	}
	if len(seen) != 4 || seen[0] == seen[1] || seen[1] != seen[2] || seen[2] != seen[3] {
		t.Fatalf("transport cooldown did not defer failed profile across requests: %#v", seen)
	}
}
