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
	return router.observeSSEBindingsWithTurn04383(ctx, accountID, headers, body, "")
}

func (router *Router) observeSSEBindingsWithTurn04383(ctx context.Context, accountID string, headers http.Header, body []byte, requestTurn string) error {
	decoder := sse.NewDecoder(len(body))
	events := decoder.Feed(body)
	events = append(events, decoder.Finish()...)
	currentTurn := strings.TrimSpace(headers.Get("x-codex-turn-state"))
	if !validHardBindingComponent04383(currentTurn) {
		currentTurn = strings.TrimSpace(requestTurn)
		if !validHardBindingComponent04383(currentTurn) {
			currentTurn = ""
		}
	}
	// Tagged response_forwarding records all precommit lookahead IDs with
	// the selected response header/request override before constructing the
	// streaming SSE tap. The tap may change its active turn when it reads an
	// event, but it cannot rewrite those pre-recorded IDs. Only when no
	// selected turn exists may prelude event fields determine each ID's turn.
	precommitSelectedTurn := currentTurn
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
			selectedTurn := currentTurn
			if precommitSelectedTurn != "" {
				selectedTurn = precommitSelectedTurn
			}
			binding := affinityKeys{previous: id}
			if selectedTurn != "" {
				binding.turn = selectedTurn
			}
			if err := router.rememberVerifiedAccountBinding(ctx, accountID, binding, router.now()); err != nil {
				return err
			}
			if selectedTurn != "" {
				router.rememberAccountTurnState(ctx, id, accountID, profileHome, selectedTurn, router.now())
				withTurn[id] = true
			}
		}
	}
	return nil
}
