package routing

import (
	"context"
	"net/http"
	"sort"
	"strings"
)

// SSELineageTracker04383 carries the tagged RuntimeSseTapState's response-ID
// and turn-state relationship across separate committed transport reads.
// A tracker belongs to exactly one HTTP response stream, never an account
// globally: unrelated requests may reuse IDs or emit different turn states.
// The precommit prefix is already observed by Router.bindSuccessfulResponse;
// Seed reconstructs its state without executing its effects a second time.
type SSELineageTracker04383 struct {
	router                                     *Router
	accountID, providerKind, profileHome, turn string
	headers                                    http.Header
	seen, withTurn                             map[string]bool
}

func (router *Router) NewSSELineageTracker04383(accountID, providerKind string, headers http.Header) *SSELineageTracker04383 {
	turn := strings.TrimSpace(headers.Get("x-codex-turn-state"))
	if !validHardBindingComponent04383(turn) {
		turn = ""
	}
	return &SSELineageTracker04383{router: router, accountID: accountID,
		providerKind: providerKind, profileHome: router.accountHome(accountID),
		headers: headers, turn: turn, seen: make(map[string]bool), withTurn: make(map[string]bool)}
}

// Seed reconstructs previously emitted event provenance. Those events were
// already observed by Router.bindSuccessfulResponse before commitment.
func (state *SSELineageTracker04383) Seed(events [][]byte) {
	for _, data := range events {
		state.advance(data, nil)
	}
}

// Observe commits one framed SSE event. Direct response-ID and token-usage
// observation remains on the established Router path; this per-stream state
// only adds inferred turn-state links for later IDs and late-turn backfill.
func (state *SSELineageTracker04383) Observe(ctx context.Context, data []byte) error {
	if err := state.router.Observe(ctx, state.accountID, state.headers, data, false, state.providerKind); err != nil {
		return err
	}
	return state.advance(data, ctx)
}

func (state *SSELineageTracker04383) advance(data []byte, ctx context.Context) error {
	keys, ids := responseAffinityWithIDs(nil, data, false)
	if validHardBindingComponent04383(keys.turn) {
		state.turn = keys.turn
	}
	// The first turn discovered after unbound responses backfills them
	// once; a later turn must not overwrite prior verified associations.
	if state.turn != "" {
		// Tagged RuntimeSseTapState backfills a BTreeSet of response IDs.
		// Sorting here also preserves deterministic durable write/eviction
		// order for multiple earlier IDs that acquire their first turn.
		unbound := make([]string, 0, len(state.seen))
		for id := range state.seen {
			if !state.withTurn[id] {
				unbound = append(unbound, id)
			}
		}
		sort.Strings(unbound)
		for _, id := range unbound {
			if ctx != nil {
				state.router.rememberAccountTurnState(ctx, id, state.accountID, state.profileHome, state.turn, state.router.now())
			}
			state.withTurn[id] = true
		}
	}
	for _, id := range ids {
		if !validHardBindingComponent04383(id) || state.seen[id] {
			continue
		}
		state.seen[id] = true
		if state.turn != "" {
			if ctx != nil {
				state.router.rememberAccountTurnState(ctx, id, state.accountID, state.profileHome, state.turn, state.router.now())
			}
			state.withTurn[id] = true
		}
	}
	return nil
}
