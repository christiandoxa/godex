package routing

import (
	"context"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// The compiled 0.438.3 SSE tap associates each newly observed response ID
// with the most recent turn-state, not a single turn selected from the first
// event in an entire buffered batch. Later states only backfill response IDs
// which did not yet have a turn-state association.
func (router *Router) observeSSEBindings04383(ctx context.Context, accountID string, headers http.Header, body []byte) error {
	decoder := sse.NewDecoder(len(body))
	events := decoder.Feed(body)
	events = append(events, decoder.Finish()...)
	currentTurn := strings.TrimSpace(headers.Get("x-codex-turn-state"))
	if !validHardBindingComponent04383(currentTurn) {
		currentTurn = ""
	}
	seenIDs := make(map[string]bool)
	withTurn := make(map[string]bool)
	profileHome := router.accountHome(accountID)
	for _, data := range events {
		// Native token-usage extraction requires a complete JSON event,
		// not an SSE wrapper with event/data framing.
		router.observeTokenUsage(ctx, accountID, data)
		keys, ids := responseAffinityWithIDs(nil, data, false)
		if validHardBindingComponent04383(keys.turn) {
			currentTurn = keys.turn
		}
		if currentTurn != "" {
			// Tagged SSE forwarding backfills the first turn-state to any
			// previously observed response ID without a bound turn.
			for id := range seenIDs {
				if !withTurn[id] {
					router.rememberAccountTurnState(ctx, id, accountID, profileHome, currentTurn, router.now())
					withTurn[id] = true
				}
			}
		}
		for _, id := range ids {
			if !validHardBindingComponent04383(id) || seenIDs[id] {
				continue
			}
			seenIDs[id] = true
			binding := affinityKeys{previous: id}
			if currentTurn != "" {
				binding.turn = currentTurn
			}
			if err := router.rememberVerifiedAccountBinding(ctx, accountID, binding, router.now()); err != nil {
				return err
			}
			if currentTurn != "" {
				router.rememberAccountTurnState(ctx, id, accountID, profileHome, currentTurn, router.now())
				withTurn[id] = true
			}
		}
	}
	return nil
}
