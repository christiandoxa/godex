package routing

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

func (store *Store) Remove(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	remove := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if len(key) != 64 {
			return errors.New("invalid routing key")
		}
		if _, err := hex.DecodeString(key); err != nil {
			return errors.New("invalid routing key")
		}
		remove[key] = struct{}{}
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing store for removal: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return fmt.Errorf("lock routing store for removal: %w", err)
	}
	defer release()
	state, err := store.readState()
	if err != nil {
		return fmt.Errorf("read routing store for removal: %w", err)
	}
	values := make([]routingentity.Binding, 0, len(state.Bindings))
	now := time.Now().Unix()
	for _, binding := range state.Bindings {
		if _, ok := remove[binding.Key]; !ok {
			values = append(values, binding)
			continue
		}
		if binding.Kind == "session" {
			if state.SessionReleaseEpochs == nil {
				state.SessionReleaseEpochs = make(map[string]int64)
			}
			epoch := max(now, binding.UpdatedUnix, state.SessionReleaseEpochs[binding.Key])
			state.SessionReleaseEpochs[binding.Key] = epoch
		}
	}
	if len(values) == len(state.Bindings) {
		return nil
	}
	state.Bindings = values
	if err := store.writeState(state); err != nil {
		return fmt.Errorf("write routing store after removal: %w", err)
	}
	return nil
}
