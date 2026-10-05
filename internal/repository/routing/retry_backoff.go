package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	maxRetryBackoffs     = 4096
	maxRetryBackoffBytes = 1 << 20
)

type retryBackoffSnapshot struct {
	Version  int                          `json:"version"`
	Backoffs []routingentity.RetryBackoff `json:"backoffs"`
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
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return nil, fmt.Errorf("read routing retry backoff store: %w", err)
	}
	maximum := now.Add(routingentity.MaxRetryBackoffDuration).Unix()
	active := make([]routingentity.RetryBackoff, 0, len(snapshot.Backoffs))
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
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return fmt.Errorf("read routing retry backoff store: %w", err)
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
	snapshot.Backoffs = retainRetryBackoffs(snapshot.Backoffs, now)
	return store.writeRetryBackoffs(snapshot.Backoffs)
}

func (store *Store) ClearRetryBackoff(ctx context.Context, accountID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	probe := routingentity.RetryBackoff{AccountID: accountID, UntilUnix: 1}
	if err := probe.Validate(); err != nil {
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
	snapshot, err := store.readRetryBackoffs()
	if err != nil {
		return fmt.Errorf("read routing retry backoff store: %w", err)
	}
	remaining := snapshot.Backoffs[:0]
	for _, backoff := range snapshot.Backoffs {
		if backoff.AccountID != accountID {
			remaining = append(remaining, backoff)
		}
	}
	if len(remaining) == len(snapshot.Backoffs) {
		return nil
	}
	return store.writeRetryBackoffs(remaining)
}

func (store *Store) readRetryBackoffs() (retryBackoffSnapshot, error) {
	snapshot := retryBackoffSnapshot{Version: 1}
	path := filepath.Join(store.root, "retry-backoff.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, fmt.Errorf("stat routing retry backoff snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxRetryBackoffBytes {
		return snapshot, errors.New("routing retry backoff snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return snapshot, fmt.Errorf("open routing retry backoff snapshot: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxRetryBackoffBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, errors.New("decode routing retry backoff snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return snapshot, errors.New("routing retry backoff snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Backoffs) > maxRetryBackoffs {
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
	return snapshot, nil
}

func (store *Store) writeRetryBackoffs(backoffs []routingentity.RetryBackoff) error {
	values := append([]routingentity.RetryBackoff(nil), backoffs...)
	sort.Slice(values, func(i, j int) bool {
		if values[i].UntilUnix != values[j].UntilUnix {
			return values[i].UntilUnix > values[j].UntilUnix
		}
		return values[i].AccountID < values[j].AccountID
	})
	content, err := json.Marshal(retryBackoffSnapshot{Version: 1, Backoffs: values})
	if err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(filepath.Join(store.root, "retry-backoff.json"), content)
	return err
}

func retainRetryBackoffs(backoffs []routingentity.RetryBackoff, now time.Time) []routingentity.RetryBackoff {
	active := make([]routingentity.RetryBackoff, 0, len(backoffs))
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
