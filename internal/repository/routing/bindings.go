package routing

import (
	"context"
	"errors"
	"path/filepath"
	"sort"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

type Store struct{ root string }

func NewStore(root string) *Store { return &Store{root: root} }

// AcquireConversation serializes first-owner selection across managed processes.
func (store *Store) AcquireConversation(ctx context.Context) (func() error, error) {
	if err := store.prepare(); err != nil {
		return nil, err
	}
	// ponytail: one first-owner lock; use bounded lock shards if contention matters.
	return lockfile.Acquire(ctx, filepath.Join(store.root, "routing-conversation.guard"))
}

func (store *Store) Load(ctx context.Context) ([]routingentity.Binding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return nil, err
	}
	defer release()
	return store.read()
}
func (store *Store) Merge(ctx context.Context, updates []routingentity.Binding) ([]routingentity.Binding, error) {
	if err := store.prepare(); err != nil {
		return nil, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return nil, err
	}
	defer release()
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	// Validate before applying a fence: even a stale invalid writer must
	// not be silently accepted as if its malformed payload were safe.
	for _, update := range updates {
		if err := update.Validate(); err != nil {
			return nil, err
		}
	}
	// Old writers may replay snapshots captured before an explicit
	// session release. Only newer session generations may re-enter.
	updates = filterReleasedSessionBindings(updates, state.SessionReleaseEpochs)
	state.Bindings, err = mergeBindingValues(state.Bindings, updates)
	if err != nil {
		return nil, err
	}
	state.Bindings, err = normalizeBindingValues(state.Bindings)
	if err != nil {
		return nil, err
	}
	if err := store.writeState(state); err != nil {
		return nil, err
	}
	return state.Bindings, nil
}

func (store *Store) MergeVerified(ctx context.Context, updates []routingentity.Binding) ([]routingentity.Binding, error) {
	if err := store.prepare(); err != nil {
		return nil, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return nil, err
	}
	defer release()
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	for _, update := range updates {
		if err := update.Validate(); err != nil {
			return nil, err
		}
	}
	// Only MergeVerified represents a new verified owner observation.
	// Advance its logical binding epoch past any release even if multiple
	// releases, rebinds, and callbacks occur in one wall-clock second.
	for index := range updates {
		entry := &updates[index]
		if entry.Kind == "session" && state.SessionReleaseEpochs != nil {
			if at, ok := state.SessionReleaseEpochs[entry.Key]; ok &&
				entry.UpdatedUnix <= at {
				if at == int64(^uint64(0)>>1) {
					return nil, errors.New("session release epoch exhausted")
				}
				entry.UpdatedUnix = at + 1
			}
		}
	}
	state.Bindings, err = mergeVerifiedBindingValues(state.Bindings, updates)
	if err != nil {
		return nil, err
	}
	state.Bindings, err = normalizeBindingValues(state.Bindings)
	if err != nil {
		return nil, err
	}
	if err := store.writeState(state); err != nil {
		return nil, err
	}
	return state.Bindings, nil
}

func mergeVerifiedBindingValues(current, updates []routingentity.Binding) ([]routingentity.Binding, error) {
	byKey := make(map[string]routingentity.Binding, len(current)+len(updates))
	for _, binding := range current {
		byKey[binding.Key] = binding
	}
	for _, binding := range updates {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		if old, ok := byKey[binding.Key]; ok && old.AccountID != binding.AccountID {
			if old.AccountID == routingentity.ConflictAccountID {
				if binding.UpdatedUnix > old.UpdatedUnix {
					old.UpdatedUnix = binding.UpdatedUnix
				}
				byKey[binding.Key] = old
				continue
			}
			binding.AccountID = routingentity.ConflictAccountID
			if old.UpdatedUnix > binding.UpdatedUnix {
				binding.UpdatedUnix = old.UpdatedUnix
			}
		}
		byKey[binding.Key] = binding
	}
	values := make([]routingentity.Binding, 0, len(byKey))
	for _, binding := range byKey {
		values = append(values, binding)
	}
	return values, nil
}

func mergeBindingValues(current, updates []routingentity.Binding) ([]routingentity.Binding, error) {
	byKey := make(map[string]routingentity.Binding, len(current)+len(updates))
	for _, binding := range current {
		byKey[binding.Key] = binding
	}
	for _, binding := range updates {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		if old, ok := byKey[binding.Key]; ok && old.AccountID != binding.AccountID {
			return nil, errors.New("routing owner conflict")
		}
		byKey[binding.Key] = binding
	}
	values := make([]routingentity.Binding, 0, len(byKey))
	for _, binding := range byKey {
		values = append(values, binding)
	}
	return values, nil
}

func normalizeBindingValues(values []routingentity.Binding) ([]routingentity.Binding, error) {
	protected := 0
	for _, binding := range values {
		if durableBinding(binding) {
			protected++
		}
	}
	if protected > routingentity.MaxBindings {
		return nil, errors.New("routing store is full; cannot discard conversation ownership")
	}
	sort.Slice(values, func(i, j int) bool { return bindingLess(values[i], values[j]) })
	if len(values) > routingentity.MaxBindings {
		values = values[:routingentity.MaxBindings]
	}
	return values, nil
}

func durableBinding(binding routingentity.Binding) bool {
	return binding.Kind == "thread" || binding.Kind == "session"
}

func bindingLess(left, right routingentity.Binding) bool {
	leftDurable, rightDurable := durableBinding(left), durableBinding(right)
	if leftDurable != rightDurable {
		return leftDurable
	}
	if left.UpdatedUnix != right.UpdatedUnix {
		return left.UpdatedUnix > right.UpdatedUnix
	}
	return left.Key < right.Key
}
