package routing

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

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
	current, err := store.read()
	if err != nil {
		return fmt.Errorf("read routing store for removal: %w", err)
	}
	values := current[:0]
	for _, binding := range current {
		if _, ok := remove[binding.Key]; !ok {
			values = append(values, binding)
		}
	}
	if len(values) == len(current) {
		return nil
	}
	if err := store.write(values); err != nil {
		return fmt.Errorf("write routing store after removal: %w", err)
	}
	return nil
}
