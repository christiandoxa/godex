package httpheader

import (
	"net/http"
	"testing"
)

func TestHopAndConnectionPolicies(t *testing.T) {
	headers := http.Header{"Connection": []string{"keep-alive, X-Local-Hop", "Upgrade"}}
	tokens := ConnectionTokens(headers)
	if !tokens["X-Local-Hop"] || !tokens["Upgrade"] {
		t.Fatalf("connection tokens = %v", tokens)
	}
	for _, name := range []string{"Connection", "Proxy-Authorization", "Transfer-Encoding", "Upgrade"} {
		if !IsHop(name) {
			t.Fatalf("hop header accepted: %s", name)
		}
	}
	if IsHop("Content-Type") || IsHop("x-codex-turn-state") {
		t.Fatal("end-to-end metadata stripped")
	}
	if !IsRequestTransport("host") || !IsRequestTransport("content-length") {
		t.Fatal("request framing accepted from caller")
	}
}

func TestConnectionTokensAcceptNonCanonicalMapKeys(t *testing.T) {
	tokens := ConnectionTokens(http.Header{"connection": []string{"X-Local-Hop"}})
	if !tokens["X-Local-Hop"] {
		t.Fatalf("lowercase connection key was ignored: %v", tokens)
	}
}
