package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	maxAuthBackoffs043714     = 4096
	maxAuthBackoffBytes043714 = 1 << 20
)

type authBackoffSnapshot043714 struct {
	Version  int                         `json:"version"`
	Backoffs []routingentity.AuthBackoff `json:"backoffs"`
	// Clear epochs are tombstones: a delayed older writer cannot resurrect
	// an auth failure previously retired by a verified quota receipt.
	Cleared map[string]int64 `json:"cleared_at_ms,omitempty"`
}

func (store *Store) readAuthBackoffs043714() (authBackoffSnapshot043714, error) {
	path := filepath.Join(store.root, "auth-backoff.json")
	state, err := readPrivateRoutingSnapshot(path, maxAuthBackoffBytes043714, decodeAuthBackoff043714)
	if err != nil {
		return state, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Cleared == nil {
		state.Cleared = make(map[string]int64)
	}
	return state, nil
}

func decodeAuthBackoff043714(payload []byte) (authBackoffSnapshot043714, error) {
	var state authBackoffSnapshot043714
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil {
		return state, errors.New("decode auth backoff snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return state, errors.New("auth backoff trailing data")
	}
	if state.Version != 1 || len(state.Backoffs) > maxAuthBackoffs043714 || len(state.Cleared) > maxAuthBackoffs043714 {
		return state, errors.New("unsupported or oversized auth backoff snapshot")
	}
	seen := make(map[string]bool, len(state.Backoffs))
	for _, entry := range state.Backoffs {
		if entry.Validate() != nil || seen[entry.AccountID] {
			return state, errors.New("invalid or duplicate auth backoff")
		}
		seen[entry.AccountID] = true
	}
	for id, epoch := range state.Cleared {
		if routingentity.ValidateAccountID(id) != nil || epoch <= 0 {
			return state, errors.New("invalid auth clear epoch")
		}
	}
	return state, nil
}

func (store *Store) writeAuthBackoffs043714(state authBackoffSnapshot043714) error {
	sort.Slice(state.Backoffs, func(i, j int) bool { return state.Backoffs[i].AccountID < state.Backoffs[j].AccountID })
	state.Version = 1
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeRoutingSnapshot(filepath.Join(store.root, "auth-backoff.json"), payload, maxAuthBackoffBytes043714,
		func(encoded []byte) error { _, err := decodeAuthBackoff043714(encoded); return err })
}

func (store *Store) LoadAuthBackoffs(ctx context.Context, now time.Time) ([]routingentity.AuthBackoff, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-auth-backoff.guard"))
	if err != nil {
		return nil, err
	}
	defer release()
	state, err := store.readAuthBackoffs043714()
	if err != nil {
		return nil, fmt.Errorf("load auth backoff: %w", err)
	}
	active := make([]routingentity.AuthBackoff, 0, len(state.Backoffs))
	for _, entry := range state.Backoffs {
		if entry.Active(now) && entry.FailedAtMillis > state.Cleared[entry.AccountID] {
			active = append(active, entry)
		}
	}
	return active, nil
}

// Compaction is safe once an entry is no longer active: the maximum auth
// penalty lasts two hours, so older clear tombstones need not be retained
// indefinitely, and expired profiles must not consume admission capacity.
func pruneAuthBackoffSnapshot043714(state *authBackoffSnapshot043714, now time.Time) {
	active := state.Backoffs[:0]
	for _, entry := range state.Backoffs {
		if entry.Active(now) && entry.FailedAtMillis > state.Cleared[entry.AccountID] {
			active = append(active, entry)
		}
	}
	state.Backoffs = active
	cutoff := now.Add(-routingentity.MaxAuthBackoffDuration).UnixMilli()
	for id, epoch := range state.Cleared {
		if epoch < cutoff {
			delete(state.Cleared, id)
		}
	}
}

func (store *Store) SetAuthBackoff(ctx context.Context, entry routingentity.AuthBackoff, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	if !entry.Active(now) || entry.FailedAtMillis > now.UnixMilli() {
		return errors.New("auth failure outside current time")
	}
	if err := store.prepare(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-auth-backoff.guard"))
	if err != nil {
		return err
	}
	defer release()
	state, err := store.readAuthBackoffs043714()
	if err != nil {
		return err
	}
	pruneAuthBackoffSnapshot043714(&state, now)
	if entry.FailedAtMillis <= state.Cleared[entry.AccountID] {
		return nil
	}
	for i, current := range state.Backoffs {
		if current.AccountID != entry.AccountID {
			continue
		}
		if current.FailedAtMillis > entry.FailedAtMillis ||
			current.FailedAtMillis == entry.FailedAtMillis && current.UntilMillis >= entry.UntilMillis {
			return nil
		}
		state.Backoffs[i] = entry
		return store.writeAuthBackoffs043714(state)
	}
	if len(state.Backoffs) >= maxAuthBackoffs043714 {
		return errors.New("auth backoff capacity exceeded")
	}
	state.Backoffs = append(state.Backoffs, entry)
	return store.writeAuthBackoffs043714(state)
}

// Clear only the exact prior auth generation, preserving any newer 401 and
// writing a merge-safe tombstone for delayed async writers.
func (store *Store) ClearAuthBackoff(ctx context.Context, id string, failedAtMillis int64, now time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if routingentity.ValidateAccountID(id) != nil || failedAtMillis <= 0 {
		return false, errors.New("invalid auth clear target")
	}
	if err := store.prepare(); err != nil {
		return false, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-auth-backoff.guard"))
	if err != nil {
		return false, err
	}
	defer release()
	state, err := store.readAuthBackoffs043714()
	if err != nil {
		return false, err
	}
	pruneAuthBackoffSnapshot043714(&state, now)
	index := -1
	for i, entry := range state.Backoffs {
		if entry.AccountID == id {
			index = i
			break
		}
	}
	if index < 0 || state.Backoffs[index].FailedAtMillis != failedAtMillis {
		return false, nil
	}
	epoch := max(now.UnixMilli(), failedAtMillis)
	if len(state.Cleared) >= maxAuthBackoffs043714 && state.Cleared[id] == 0 {
		return false, errors.New("auth tombstone capacity exceeded")
	}
	state.Cleared[id] = max(epoch, state.Cleared[id])
	state.Backoffs = append(state.Backoffs[:index], state.Backoffs[index+1:]...)
	if err := store.writeAuthBackoffs043714(state); err != nil {
		return false, err
	}
	return true, nil
}
