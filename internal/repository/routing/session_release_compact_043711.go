package routing

import (
	"errors"
	"sort"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

const sessionReleaseRetiredRetention043711 = int64(7 * 24 * 60 * 60)

// Tagged Prodex 0.437.11 retains release epochs for live session bindings,
// retains retired epochs for seven days, and evicts the oldest retired
// entries when the default 2048*4 fence budget is exceeded. A release fence
// is never removed while a newer owner binding remains live.
func compactSessionReleaseFences043711(state *snapshot, now int64, limit int) error {
	if len(state.SessionReleaseEpochs) == 0 {
		return nil
	}
	state.Bindings = filterReleasedSessionBindings(state.Bindings, state.SessionReleaseEpochs)
	live := make(map[string]bool)
	for _, binding := range state.Bindings {
		if binding.Kind == "session" {
			live[binding.Key] = true
		}
	}
	for key, epoch := range state.SessionReleaseEpochs {
		if live[key] {
			continue
		}
		if now > sessionReleaseRetiredRetention043711 &&
			epoch < now-sessionReleaseRetiredRetention043711 {
			delete(state.SessionReleaseEpochs, key)
		}
	}
	if len(state.SessionReleaseEpochs) <= limit {
		return nil
	}
	type retired struct {
		key   string
		epoch int64
	}
	candidates := make([]retired, 0, len(state.SessionReleaseEpochs))
	for key, epoch := range state.SessionReleaseEpochs {
		if !live[key] {
			candidates = append(candidates, retired{key: key, epoch: epoch})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].epoch == candidates[j].epoch {
			return candidates[i].key < candidates[j].key
		}
		return candidates[i].epoch < candidates[j].epoch
	})
	excess := len(state.SessionReleaseEpochs) - limit
	for _, item := range candidates[:min(excess, len(candidates))] {
		delete(state.SessionReleaseEpochs, item.key)
	}
	if len(state.SessionReleaseEpochs) > limit || len(state.SessionReleaseEpochs) > routingentity.MaxBindings {
		return errors.New("routing session release fence storage is full")
	}
	return nil
}
