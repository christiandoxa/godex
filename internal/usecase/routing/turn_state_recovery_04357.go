package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func requestHasReconstructableFullHistory(request proxymodel.Request) bool {
	// Tagged runtime_request_has_reconstructable_full_history parses the
	// whole request as serde_json::Value before evaluating its input shape.
	if _, valid := decodeNativeUsageJSON(request.Body); !valid {
		return false
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(request.Body, &body) != nil {
		return false
	}
	var input []json.RawMessage
	return json.Unmarshal(body["input"], &input) == nil && reconstructableFullHistory(input)
}

func requestWithoutTurnState(request proxymodel.Request) proxymodel.Request {
	request.Header = request.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	// net/http canonicalizes Del's argument, not every existing map key.
	// A caller-built Header can contain several ASCII-case variants; remove
	// all of them to match Prodex's tagged outbound header filter.
	for key := range request.Header {
		if len(key) == len("x-codex-turn-state") && strings.EqualFold(key, "x-codex-turn-state") {
			delete(request.Header, key)
		}
	}

	// Keep the client's input and unknown fields opaque: decoding into
	// map[string]any would round large JSON integers through float64 and
	// silently rewrite full-history tool arguments during 0.438.3 recovery.
	// Tagged runtime_request_without_turn_state leaves the original body
	// untouched when serde_json cannot parse the complete JSON document.
	if _, valid := decodeNativeUsageJSON(request.Body); !valid {
		return request
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(request.Body, &body) != nil {
		return request
	}
	removed := false
	if _, ok := body["x-codex-turn-state"]; ok {
		delete(body, "x-codex-turn-state")
		removed = true
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(body["client_metadata"], &metadata) == nil {
		if _, found := metadata["x-codex-turn-state"]; found {
			delete(metadata, "x-codex-turn-state")
			if encoded, err := json.Marshal(metadata); err == nil {
				body["client_metadata"] = encoded
				removed = true
			}
		}
	}
	if !removed {
		return request
	}
	if encoded, err := json.Marshal(body); err == nil {
		request.Body = encoded
	}
	return request
}

func (store *affinityStore) deadTurnState(value string, now time.Time) bool {
	return store.deadTurnStateContext(context.Background(), value, now)
}

func (store *affinityStore) deadTurnStateContext(ctx context.Context, value string, now time.Time) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	key := affinityDigest("turn", value)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	store.pruneContinuationStatusesLocked(now)
	if _, live := store.values[key]; live {
		return false
	}
	status, ok := store.statuses[key]
	if !ok || status.kind != "turn_state" || status.state != continuationDead {
		return false
	}
	if _, checked := store.turnBindingChecks[key]; !checked {
		if store.turnBindingChecks == nil {
			store.turnBindingChecks = make(map[string]struct{})
		}
		if len(store.turnBindingChecks) >= affinityMaxValues {
			for oldest := range store.turnBindingChecks {
				delete(store.turnBindingChecks, oldest)
				break
			}
		}
		store.turnBindingChecks[key] = struct{}{}
		_ = store.loadMissingLocked(ctx, affinityKeys{turn: value}, now)
	}
	_, live := store.values[key]
	return !live
}

func (store *affinityStore) releaseOwnedDead(
	ctx context.Context,
	accountID string,
	keys affinityKeys,
	now time.Time,
) error {
	// The tagged release plan preserves session ownership beside a hard identity.
	if strings.TrimSpace(keys.previous) != "" || strings.TrimSpace(keys.turn) != "" {
		keys.session = ""
	}
	entries := keys.entries()
	if strings.TrimSpace(accountID) == "" || len(entries) == 0 {
		return nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.loadMissingLocked(ctx, keys, now); err != nil {
		return err
	}

	remove := make([]bindingEntry, 0, len(entries))
	for _, entry := range entries {
		if current, ok := store.values[entry.Key]; ok && current.accountID == accountID {
			remove = append(remove, bindingEntry{Kind: entry.Kind, Key: entry.Key})
		}
	}
	if len(remove) == 0 {
		return nil
	}
	for _, entry := range remove {
		if kind := continuationStatusKind(entry.Kind); kind != "" {
			store.markContinuationDeadLocked(kind, entry.Key, now)
			if err := store.persistContinuationDeadLocked(ctx, kind, entry.Key, now); err != nil {
				return err
			}
		}
	}
	if remover, ok := store.repository.(affinityBindingRemover); ok && store.writesEnabled() {
		keys := make([]string, 0, len(remove))
		for _, entry := range remove {
			keys = append(keys, entry.Key)
		}
		if err := remover.Remove(ctx, keys); err != nil {
			return err
		}
	}
	for _, entry := range remove {
		delete(store.values, entry.Key)
	}
	store.pruneContinuationStatusesLocked(now)
	return nil
}

func (router *Router) scrubDeadTurnState(request proxymodel.Request) proxymodel.Request {
	return router.scrubDeadTurnStateContext(context.Background(), request)
}

func (router *Router) scrubDeadTurnStateContext(ctx context.Context, request proxymodel.Request) proxymodel.Request {
	keys := requestRoutingAffinity(request)
	if keys.turn == "" || !router.affinity.deadTurnStateContext(ctx, keys.turn, router.now()) {
		return request
	}
	return requestWithoutTurnState(request)
}

func (router *Router) turnStateOwnedBy(
	ctx context.Context,
	turnState, accountID string,
) (bool, error) {
	turnState = strings.TrimSpace(turnState)
	if turnState == "" || strings.TrimSpace(accountID) == "" {
		return false, nil
	}
	owner, err := router.affinity.owner(ctx, affinityKeys{turn: turnState}, router.now())
	if err != nil {
		return false, err
	}
	return owner == accountID, nil
}
