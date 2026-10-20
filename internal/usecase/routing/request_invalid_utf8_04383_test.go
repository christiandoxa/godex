package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepository "github.com/christiandoxa/godex/internal/repository/routing"
)

// Prodex uses serde_json::from_slice for inbound request semantics: invalid
// UTF-8 is not a valid JSON request identity. Go's json.Unmarshal replaces
// these bytes with U+FFFD; that replacement must not steal a verified owner.
func TestProdex04383InvalidUTF8RequestDoesNotClaimHardAffinity(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, tc := range []struct {
			name, kind, field string
		}{
			{"session", "session", "session_id"},
			{"previous", "previous", "previous_response_id"},
		} {
			name := "memory/" + tc.name
			if durable {
				name = "durable/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				original := []byte(`{"model":"gpt-6","` + tc.field + `":"prefix-`)
				original = append(original, 0xff)
				original = append(original, []byte(`","input":[{"role":"user","content":"independent"}]}`)...)
				expectedOwnerKey := "prefix-\ufffd"
				gateway := &standardOnlyRoutingGateway{execute: func() *proxymodel.Response {
					return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(`{"id":"resp-invalid-utf8","object":"response","output":[]}`))}
				}}
				config := Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
					Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }}
				if durable {
					config.Bindings = routingrepository.NewStore(t.TempDir())
				}
				router, err := NewRouter(config)
				if err != nil {
					t.Fatal(err)
				}
				defer router.Close()
				key := affinityKeys{}
				if tc.kind == "session" {
					key.session = expectedOwnerKey
				} else {
					key.previous = expectedOwnerKey
				}
				if err := router.affinity.remember(t.Context(), websocketQuotaOwnerB, key, router.now()); err != nil {
					t.Fatal(err)
				}
				request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: original,
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				if keys := requestRoutingAffinity(request); keys.previous != "" || keys.session != "" {
					t.Fatalf("malformed JSON unexpectedly gained continuation affinity: %+v", keys)
				}
				result, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer result.Close()
				if result.Result.AccountID != websocketQuotaOwnerA || result.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("invalid UTF8 gained foreign owner: account=%q status=%d", result.Result.AccountID, result.Result.Response.StatusCode)
				}
				store := router.affinity
				if durable {
					store = newAffinityStore()
					store.repository = config.Bindings
				}
				owner, err := store.owner(t.Context(), key, router.now())
				if err != nil || owner != websocketQuotaOwnerB {
					t.Fatalf("original verified owner changed: owner=%q err=%v", owner, err)
				}
			})
		}
	}
}

func TestProdex04383WebSocketInvalidUTF8NeverPromotesRequestMetadata(t *testing.T) {
	for _, tc := range []struct{ name, field string }{
		{"session", "session_id"}, {"turn", "x-codex-turn-state"}, {"previous", "previous_response_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(`{"type":"response.create","` + tc.field + `":"bad-`)
			original = append(original, 0xfe)
			original = append(original, []byte(`","input":[]}`)...)
			request := proxymodel.Request{WebSocketMessage: true, Header: make(http.Header), Body: original}
			metadata := parseWebSocketRequestMetadata(request)
			if metadata.sessionID != "" || metadata.turnState != "" || metadata.previousResponseID != "" || metadata.requiresPreviousResponseAffinity {
				t.Fatalf("invalid websocket message gained continuation metadata: %+v", metadata)
			}
		})
	}
}

func TestProdex04383InvalidUTF8PromptCacheAndUsageCannotMintEvidence(t *testing.T) {
	malformed := []byte(`{"prompt_cache_key":"cache-`)
	malformed = append(malformed, 0xff)
	malformed = append(malformed, []byte(`","usage":{"cached_input_tokens":90},"type":"response.completed"}`)...)
	if got := promptCacheKeyFromBody(malformed); got != "" {
		t.Fatalf("malformed request became cache owner key %q", got)
	}
	if counts, ok := terminalTokenUsage(malformed); ok || counts.CachedInputTokens != 0 {
		t.Fatalf("malformed JSON gained usage marker: %+v valid=%t", counts, ok)
	}
	// Deliberately encoded U+FFFD is legitimate UTF-8; only invalid raw
	// bytes are excluded, not a user explicitly naming the replacement rune.
	valid := []byte(`{"prompt_cache_key":"cache-\ufffd","type":"response.completed","usage":{"cached_input_tokens":90}}`)
	if got := promptCacheKeyFromBody(valid); got != "cache-\ufffd" {
		t.Fatalf("valid unicode prompt cache key=%q", got)
	}
	if counts, ok := terminalTokenUsage(valid); !ok || counts.CachedInputTokens != 90 {
		t.Fatalf("valid unicode usage=%+v valid=%t", counts, ok)
	}
}
