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
	protected := 0
	for _, binding := range values {
		if binding.Kind == "thread" || binding.Kind == "session" {
			protected++
		}
	}
	if protected > routingentity.MaxBindings {
		return nil, errors.New("routing store is full; cannot discard conversation ownership")
	}
	sort.Slice(values, func(i, j int) bool {
		left := values[i].Kind == "thread" || values[i].Kind == "session"
		right := values[j].Kind == "thread" || values[j].Kind == "session"
		if left != right {
			return left
		}
		if values[i].UpdatedUnix != values[j].UpdatedUnix {
			return values[i].UpdatedUnix > values[j].UpdatedUnix
		}
		return values[i].Key < values[j].Key
	})
	if len(values) > routingentity.MaxBindings {
		values = values[:routingentity.MaxBindings]
	}
	content, err := json.Marshal(snapshot{Version: 1, Bindings: values})
	if err != nil {
		return nil, err
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(store.root, "routing.json"), content); err != nil {
		return nil, err
	}
	return values, nil
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
	path := filepath.Join(store.root, "routing.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return nil, errors.New("routing snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, (2<<20)+1))
	decoder.DisallowUnknownFields()
	var value snapshot
	if decoder.Decode(&value) != nil {
		return nil, errors.New("decode routing snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("routing snapshot has trailing data")
	}
	if value.Version != 1 {
		return nil, errors.New("unsupported routing snapshot version")
	}
	if len(value.Bindings) > routingentity.MaxBindings {
		return nil, errors.New("routing snapshot exceeds binding limit")
	}
	values := make([]routingentity.Binding, 0, len(value.Bindings))
	seen := make(map[string]bool, len(value.Bindings))
	for _, binding := range value.Bindings {
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		if seen[binding.Key] {
			return nil, errors.New("routing snapshot has duplicate keys")
		}
		seen[binding.Key] = true
		if binding.Kind == "thread" || binding.Kind == "session" || binding.UpdatedUnix > time.Now().Unix()-routingentity.RetentionSeconds {
			values = append(values, binding)
		}
	}
	return values, nil
}
