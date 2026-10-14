package routing

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"path/filepath"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

// ReleaseSessions is the atomic, durable counterpart of an explicit native
// session release. A previously removed or missing session must still publish
// a logical release fence: otherwise a lagging worker can resurrect an owner
// from an old snapshot after the release has already completed.
func (store *Store) ReleaseSessions(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if len(key) != 64 {
			return errors.New("invalid session release key")
		}
		if _, err := hex.DecodeString(key); err != nil {
			return errors.New("invalid session release digest")
		}
		unique[key] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return err
	}
	defer release()
	state, err := store.readState()
	if err != nil {
		return err
	}
	// Capacity is checked after the seven-day retired-fence compaction,
	// so old barriers do not permanently prevent an explicit release.
	if state.SessionReleaseEpochs == nil {
		state.SessionReleaseEpochs = make(map[string]int64, len(unique))
	}
	now := time.Now().Unix()
	for key := range unique {
		epoch := now
		if previous, ok := state.SessionReleaseEpochs[key]; ok && previous >= epoch {
			if previous == math.MaxInt64 {
				return errors.New("routing session release epoch exhausted")
			}
			epoch = previous + 1
		}
		for _, binding := range state.Bindings {
			if binding.Kind == "session" && binding.Key == key && binding.UpdatedUnix >= epoch {
				if binding.UpdatedUnix == math.MaxInt64 {
					return errors.New("routing session release epoch exhausted")
				}
				epoch = binding.UpdatedUnix + 1
			}
		}
		state.SessionReleaseEpochs[key] = epoch
	}
	active := make([]routingentity.Binding, 0, len(state.Bindings))
	for _, binding := range state.Bindings {
		if binding.Kind == "session" {
			if _, requested := unique[binding.Key]; requested {
				continue
			}
		}
		active = append(active, binding)
	}
	state.Bindings = active
	return store.writeState(state)
}
