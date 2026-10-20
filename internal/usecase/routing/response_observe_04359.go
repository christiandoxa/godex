package routing

import (
	"context"
	"net/http"
	"strings"
)

func (router *Router) Observe(ctx context.Context, accountID string, headers http.Header, body []byte, stream bool, providerKinds ...string) error {
	router.observeTokenUsage(ctx, accountID, body)
	keys, responseIDs := responseAffinityWithIDs(headers, body, stream)
	// Prodex 0.438.3 verifies session ownership from request semantics on
	// successful attempts. A response body, SSE event or upstream header
	// cannot mint a new session/thread owner simply by naming one.
	keys.session, keys.thread = "", ""
	var extraIDs []string
	for _, id := range responseIDs {
		if id != keys.previous {
			extraIDs = append(extraIDs, id)
		}
	}
	if len(providerKinds) > 0 && strings.EqualFold(strings.TrimSpace(providerKinds[0]), "copilot") {
		// Copilot accepts native responseId and message.id shapes as well as
		// OpenAI response IDs. Generic OpenAI event IDs must remain ignored.
		ids := copilotResponseIDs(body, stream)
		keys.previous, extraIDs = "", nil
		if len(ids) > 0 {
			keys.previous, extraIDs = ids[0], ids[1:]
		}
	}
	// Record only tagged RuntimeHardBindingIdentity components; a large
	// or control-bearing upstream ID cannot mint a verified owner or
	// create a cross-turn state sidecar.
	if !validHardBindingComponent04383(keys.previous) {
		keys.previous = ""
	}
	if !validHardBindingComponent04383(keys.turn) {
		keys.turn = ""
	}
	now := router.now()
	if err := router.rememberVerifiedAccountBinding(ctx, accountID, keys, now); err != nil {
		return err
	}
	router.rememberAccountTurnState(ctx, keys.previous, accountID, router.accountHome(accountID), keys.turn, now)
	for _, id := range extraIDs {
		if !validHardBindingComponent04383(id) {
			continue
		}
		if err := router.rememberVerifiedAccountBinding(ctx, accountID, affinityKeys{previous: id}, now); err != nil {
			return err
		}
		// An alternate response ID retains the same observed turn state.
		router.rememberAccountTurnState(ctx, id, accountID, router.accountHome(accountID), keys.turn, now)
	}
	return nil
}
