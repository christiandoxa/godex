package routing

import (
	"context"
	"encoding/json"
	"errors"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Store struct{ root string }
type snapshot struct {
	Version  int                     `json:"version"`
	Bindings []routingentity.Binding `json:"bindings"`
}

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
	current, err := store.read()
	if err != nil {
		return nil, err
	}
	values, err := mergeBindingValues(current, updates)
	if err != nil {
		return nil, err
	}
	values, err = normalizeBindingValues(values)
	if err != nil {
		return nil, err
	}
	if err := store.write(values); err != nil {
		return nil, err
	}
	return values, nil
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
	current, err := store.read()
	if err != nil {
		return nil, err
	}
	values, err := mergeVerifiedBindingValues(current, updates)
	if err != nil {
		return nil, err
	}
	values, err = normalizeBindingValues(values)
	if err != nil {
		return nil, err
	}
	if err := store.write(values); err != nil {
		return nil, err
	}
	return values, nil
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

func (store *Store) write(values []routingentity.Binding) error {
	content, err := json.Marshal(snapshot{Version: 1, Bindings: values})
	if err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(filepath.Join(store.root, "routing.json"), content)
	return err
}

func (store *Store) prepare() error {
	if !filepath.IsAbs(store.root) || store.root == filepath.Dir(store.root) {
		return errors.New("invalid routing home")
	}
	if err := os.MkdirAll(store.root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(store.root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("routing home must be a real directory")
	}
	return os.Chmod(store.root, 0700)
}
func (store *Store) read() ([]routingentity.Binding, error) {
	file, found, err := store.openSnapshot()
	if err != nil || !found {
		return nil, err
	}
	defer file.Close()
	value, err := decodeSnapshot(file)
	if err != nil {
		return nil, err
	}
	return filterSnapshotBindings(value.Bindings)
}

func (store *Store) openSnapshot() (*os.File, bool, error) {
	path := filepath.Join(store.root, "routing.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, false, errors.New("routing snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	return file, err == nil, err
}

func decodeSnapshot(file *os.File) (snapshot, error) {
	decoder := json.NewDecoder(io.LimitReader(file, (2<<20)+1))
	decoder.DisallowUnknownFields()
	var value snapshot
	if decoder.Decode(&value) != nil {
		return snapshot{}, errors.New("decode routing snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return snapshot{}, errors.New("routing snapshot has trailing data")
	}
	if value.Version != 1 {
		return snapshot{}, errors.New("unsupported routing snapshot version")
	}
	if len(value.Bindings) > routingentity.MaxBindings {
		return snapshot{}, errors.New("routing snapshot exceeds binding limit")
	}
	return value, nil
}

func filterSnapshotBindings(bindings []routingentity.Binding) ([]routingentity.Binding, error) {
	values := make([]routingentity.Binding, 0, len(bindings))
	seen := make(map[string]bool, len(bindings))
	cutoff := time.Now().Unix() - routingentity.RetentionSeconds
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		if seen[binding.Key] {
			return nil, errors.New("routing snapshot has duplicate keys")
		}
		seen[binding.Key] = true
		if durableBinding(binding) || binding.UpdatedUnix > cutoff {
			values = append(values, binding)
		}
	}
	return values, nil
}
