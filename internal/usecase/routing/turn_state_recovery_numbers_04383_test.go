package routing

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// A Prodex 0.438.3 full-history transport recovery must strip only the
// failed turn-state binding. Opaque model input and client metadata may carry
// integers outside IEEE-754's exact range; re-decoding them through float64
// changes a user's original tool arguments on the alternate profile.
func TestProdex04383TurnStateRecoveryPreservesOpaqueNumericHistory(t *testing.T) {
	const original = `{"type":"response.create","x-codex-turn-state":"turn-old","input":[{"type":"function_call","call_id":"c1","arguments":{"sequence":9007199254740993,"signed":-9223372036854775808}}],"client_metadata":{"x-codex-turn-state":"turn-old","sequence":18446744073709551615,"extra":{"nested":9007199254740993}},"max_output_tokens":9007199254740993}`
	request := proxymodel.Request{Header: make(http.Header), Body: []byte(original)}
	request.Header.Set("x-codex-turn-state", "turn-old")
	request.Header.Set("X-Request-Id", "synthetic-id")

	clean := requestWithoutTurnState(request)
	if clean.Header.Get("x-codex-turn-state") != "" || clean.Header.Get("X-Request-Id") != "synthetic-id" {
		t.Fatalf("recovery changed unrelated request headers: %v", clean.Header)
	}
	if request.Header.Get("x-codex-turn-state") != "turn-old" || string(request.Body) != original {
		t.Fatal("recovery mutated original request")
	}
	if strings.Contains(string(clean.Body), "turn-old") {
		t.Fatalf("replay leaked failed turn-state: %s", clean.Body)
	}
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(request.Body, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(clean.Body, &after); err != nil {
		t.Fatal(err)
	}
	if _, found := after["x-codex-turn-state"]; found {
		t.Fatal("top-level turn state retained")
	}
	for _, key := range []string{"input", "max_output_tokens"} {
		if string(after[key]) != string(before[key]) {
			t.Fatalf("%s changed across safe transport replay: before=%s after=%s", key, before[key], after[key])
		}
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(after["client_metadata"], &metadata); err != nil {
		t.Fatal(err)
	}
	if _, found := metadata["x-codex-turn-state"]; found {
		t.Fatal("nested turn state retained")
	}
	if string(metadata["sequence"]) != "18446744073709551615" || string(metadata["extra"]) != `{"nested":9007199254740993}` {
		t.Fatalf("opaque metadata numbers changed: %s", after["client_metadata"])
	}
}

// The Go Header map can contain both canonical and noncanonical keys when a
// caller constructs it directly. The tagged Prodex adapter drops *all* ASCII
// case-insensitive variants before handing history to another profile; merely
// calling http.Header.Del would leave the lowercase and uppercase aliases.
func TestProdex04383TransportReplayRemovesEveryTurnStateHeaderCase(t *testing.T) {
	request := proxymodel.Request{Header: http.Header{
		"X-Codex-Turn-State": {"stale-canonical"},
		"x-codex-turn-state": {"stale-lowercase"},
		"X-CODEX-TURN-STATE": {"stale-uppercase"},
		"x-cOdeX-tuRN-stAte": {"stale-mixed"},
		"X-Request-Id":       {"synthetic-request"},
	}, Body: []byte(`{"type":"response.create","input":[]}`)}

	clean := requestWithoutTurnState(request)
	for key := range clean.Header {
		if strings.EqualFold(key, "x-codex-turn-state") {
			t.Fatalf("recovered request leaked old owner via %q: %v", key, clean.Header)
		}
	}
	if clean.Header.Get("X-Request-Id") != "synthetic-request" ||
		len(request.Header) != 5 || len(clean.Header) != 1 {
		t.Fatalf("unrelated header lost or original mutated: original=%v replay=%v", request.Header, clean.Header)
	}
}
