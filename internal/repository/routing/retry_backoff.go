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
	maxRetryBackoffs            = 4096
	maxRetryBackoffUpdates      = 4096
	maxRetryBackoffBytes        = 1 << 20
	retryBackoffUpdateRetention = 14 * 24 * time.Hour
)

type retryBackoffSnapshot struct {
	Version   int                          `json:"version"`
	Backoffs  []routingentity.RetryBackoff `json:"backoffs"`
	UpdatedAt map[string]int64             `json:"updated_at,omitempty"`
}

func (store *Store) LoadRetryBackoffs(ctx context.Context, now time.Time) ([]routingentity.RetryBackoff, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing retry backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-retry-backoff.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing retry backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return nil, fmt.Errorf("read routing retry backoff store: %w", err)
	}
	maximum := now.Add(routingentity.MaxRetryBackoffDuration).Unix()
	active := snapshot.Backoffs[:0]
	for _, backoff := range snapshot.Backoffs {
		if backoff.UntilUnix > maximum {
			return nil, errors.New("routing retry backoff exceeds maximum duration")
		}
		if backoff.UntilUnix > now.Unix() {
			active = append(active, backoff)
		}
	}
	return active, nil
}

func (store *Store) SetRetryBackoff(ctx context.Context, backoff routingentity.RetryBackoff, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := backoff.Validate(); err != nil {
		return err
	}
	if backoff.UntilUnix <= now.Unix() || backoff.UntilUnix > now.Add(routingentity.MaxRetryBackoffDuration).Unix() {
		return errors.New("routing retry backoff exceeds maximum duration")
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing retry backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-retry-backoff.guard"))
	if err != nil {
		return fmt.Errorf("lock routing retry backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return fmt.Errorf("read routing retry backoff store: %w", err)
	}
	mutation := now.UnixMilli()
	if snapshot.UpdatedAt != nil && snapshot.UpdatedAt[backoff.AccountID] > mutation {
		return nil
	}
	replaced := false
	for index, current := range snapshot.Backoffs {
		if current.AccountID == backoff.AccountID {
			snapshot.Backoffs[index] = backoff
			replaced = true
			break
		}
	}
	if !replaced {
		snapshot.Backoffs = append(snapshot.Backoffs, backoff)
	}
	if snapshot.UpdatedAt == nil {
		snapshot.UpdatedAt = make(map[string]int64)
	}
	snapshot.UpdatedAt[backoff.AccountID] = mutation
	snapshot.Backoffs = retainRetryBackoffs(snapshot.Backoffs, now)
	snapshot.UpdatedAt = retainRetryBackoffUpdates(snapshot.UpdatedAt, now)
	return store.writeRetryBackoffs(snapshot.Backoffs, snapshot.UpdatedAt)
}

func (store *Store) ClearRetryBackoff(ctx context.Context, accountID string, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := routingentity.ValidateAccountID(accountID); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing retry backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-retry-backoff.guard"))
	if err != nil {
		return fmt.Errorf("lock routing retry backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return fmt.Errorf("read routing retry backoff store: %w", err)
	}
	mutation := now.UnixMilli()
	if snapshot.UpdatedAt != nil && snapshot.UpdatedAt[accountID] > mutation {
		return nil
	}
	remaining := snapshot.Backoffs[:0]
	for _, backoff := range snapshot.Backoffs {
		if backoff.AccountID != accountID {
			remaining = append(remaining, backoff)
		}
	}
	if snapshot.UpdatedAt == nil {
		snapshot.UpdatedAt = make(map[string]int64)
	}
	snapshot.UpdatedAt[accountID] = mutation
	snapshot.UpdatedAt = retainRetryBackoffUpdates(snapshot.UpdatedAt, now)
	return store.writeRetryBackoffs(remaining, snapshot.UpdatedAt)
}

func (store *Store) readRetryBackoffs() (retryBackoffSnapshot, error) {
	path := filepath.Join(store.root, "retry-backoff.json")
	snapshot, err := readRoutingSnapshot(path, maxRetryBackoffBytes, decodeRetryBackoffs)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	if snapshot.UpdatedAt == nil {
		snapshot.UpdatedAt = make(map[string]int64)
	}
	return snapshot, nil
}

func decodeRetryBackoffs(content []byte) (retryBackoffSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	snapshot := retryBackoffSnapshot{UpdatedAt: make(map[string]int64)}
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, errors.New("decode routing retry backoff snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return snapshot, errors.New("routing retry backoff snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Backoffs) > maxRetryBackoffs || len(snapshot.UpdatedAt) > maxRetryBackoffUpdates {
		return snapshot, errors.New("unsupported or oversized routing retry backoff snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Backoffs))
	for _, backoff := range snapshot.Backoffs {
		if err := backoff.Validate(); err != nil {
			return snapshot, err
		}
		if seen[backoff.AccountID] {
			return snapshot, errors.New("routing retry backoff snapshot has duplicate accounts")
		}
		seen[backoff.AccountID] = true
	}
	for accountID, updatedAt := range snapshot.UpdatedAt {
		if err := routingentity.ValidateAccountID(accountID); err != nil || updatedAt <= 0 {
			return snapshot, errors.New("routing retry backoff snapshot has invalid update metadata")
		}
	}
	return snapshot, nil
}

func (store *Store) writeRetryBackoffs(backoffs []routingentity.RetryBackoff, updatedAt map[string]int64) error {
	sort.Slice(backoffs, func(i, j int) bool {
		if backoffs[i].UntilUnix != backoffs[j].UntilUnix {
			return backoffs[i].UntilUnix > backoffs[j].UntilUnix
		}
		return backoffs[i].AccountID < backoffs[j].AccountID
	})
	content, err := json.Marshal(retryBackoffSnapshot{Version: 1, Backoffs: backoffs, UpdatedAt: updatedAt})
	if err != nil {
		return err
	}
	path := filepath.Join(store.root, "retry-backoff.json")
	return writeRoutingSnapshot(path, content, maxRetryBackoffBytes, func(content []byte) error {
		_, err := decodeRetryBackoffs(content)
		return err
	})
}

func retainRetryBackoffs(backoffs []routingentity.RetryBackoff, now time.Time) []routingentity.RetryBackoff {
	active := backoffs[:0]
	for _, backoff := range backoffs {
		if backoff.UntilUnix > now.Unix() {
			active = append(active, backoff)
		}
	}
	if len(active) > maxRetryBackoffs {
		sort.Slice(active, func(i, j int) bool {
			if active[i].UntilUnix != active[j].UntilUnix {
				return active[i].UntilUnix > active[j].UntilUnix
			}
			return active[i].AccountID < active[j].AccountID
		})
		active = active[:maxRetryBackoffs]
	}
	return active
}

func retainRetryBackoffUpdates(updatedAt map[string]int64, now time.Time) map[string]int64 {
	if len(updatedAt) == 0 {
		return updatedAt
	}
	cutoff := now.Add(-retryBackoffUpdateRetention).UnixMilli()
	for accountID, mutation := range updatedAt {
		if mutation < cutoff {
			delete(updatedAt, accountID)
		}
	}
	if len(updatedAt) <= maxRetryBackoffUpdates {
		return updatedAt
	}
	type update struct {
		accountID string
		mutation  int64
	}
	values := make([]update, 0, len(updatedAt))
	for accountID, mutation := range updatedAt {
		values = append(values, update{accountID: accountID, mutation: mutation})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].mutation != values[j].mutation {
			return values[i].mutation > values[j].mutation
		}
		return values[i].accountID < values[j].accountID
	})
	for _, value := range values[maxRetryBackoffUpdates:] {
		delete(updatedAt, value.accountID)
	}
	return updatedAt
}
