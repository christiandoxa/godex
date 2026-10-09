package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	maxContinuationStatuses      = 4096
	maxContinuationStatusBytes   = 512 << 10
	continuationStatusRetention  = 15 * time.Minute
	continuationStatusSnapshotV1 = 1
)

type continuationStatusSnapshot struct {
	Version  int                                `json:"version"`
	Statuses []routingentity.ContinuationStatus `json:"statuses"`
}

// LoadContinuationStatuses returns terminal continuation statuses that have not
// exceeded their replay-suppression grace period.
func (store *Store) LoadContinuationStatuses(
	ctx context.Context,
	now time.Time,
) ([]routingentity.ContinuationStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare continuation status store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock continuation status store: %w", err)
	}
	defer release()
	statuses, err := store.readContinuationStatuses()
	if err != nil {
		return nil, fmt.Errorf("read continuation status store: %w", err)
	}
	return retainContinuationStatuses(statuses, now), nil
}

// SaveContinuationStatus atomically records a terminal continuation status.
// Keys are already SHA-256 digests, so no opaque continuation value is written.
func (store *Store) SaveContinuationStatus(
	ctx context.Context,
	status routingentity.ContinuationStatus,
	now time.Time,
) error {
	if err := status.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare continuation status store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing.guard"))
	if err != nil {
		return fmt.Errorf("lock continuation status store: %w", err)
	}
	defer release()
	statuses, err := store.readContinuationStatuses()
	if err != nil {
		return fmt.Errorf("read continuation status store: %w", err)
	}
	index := continuationStatusIndex(statuses, status.Kind, status.Key)
	if index >= 0 {
		if statuses[index].UpdatedUnix >= status.UpdatedUnix {
			return nil
		}
		statuses[index] = status
	} else {
		statuses = append(statuses, status)
	}
	statuses = retainContinuationStatuses(statuses, now)
	content, err := json.Marshal(continuationStatusSnapshot{
		Version: continuationStatusSnapshotV1, Statuses: statuses,
	})
	if err != nil {
		return err
	}
	if len(content) > maxContinuationStatusBytes {
		return errors.New("continuation status snapshot exceeds size limit")
	}
	if _, err := fileutil.AtomicWrite(store.continuationStatusPath(), content); err != nil {
		return fmt.Errorf("write continuation status store: %w", err)
	}
	return nil
}

func (store *Store) readContinuationStatuses() ([]routingentity.ContinuationStatus, error) {
	path := store.continuationStatusPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat continuation status snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxContinuationStatusBytes ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return nil, errors.New("continuation status snapshot must be a private regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open continuation status snapshot: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxContinuationStatusBytes+1))
	decoder.DisallowUnknownFields()
	var snapshot continuationStatusSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, errors.New("decode continuation status snapshot")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("continuation status snapshot has trailing data")
	}
	if snapshot.Version != continuationStatusSnapshotV1 || len(snapshot.Statuses) > maxContinuationStatuses {
		return nil, errors.New("unsupported or oversized continuation status snapshot")
	}
	seen := make(map[string]struct{}, len(snapshot.Statuses))
	for _, status := range snapshot.Statuses {
		if err := status.Validate(); err != nil {
			return nil, err
		}
		identity := status.Kind + ":" + status.Key
		if _, exists := seen[identity]; exists {
			return nil, errors.New("continuation status snapshot has duplicate entries")
		}
		seen[identity] = struct{}{}
	}
	return snapshot.Statuses, nil
}

func retainContinuationStatuses(
	statuses []routingentity.ContinuationStatus,
	now time.Time,
) []routingentity.ContinuationStatus {
	cutoff := now.Add(-continuationStatusRetention).Unix()
	active := statuses[:0]
	for _, status := range statuses {
		if status.UpdatedUnix > cutoff {
			active = append(active, status)
		}
	}
	if len(active) <= maxContinuationStatuses {
		return active
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].UpdatedUnix != active[j].UpdatedUnix {
			return active[i].UpdatedUnix > active[j].UpdatedUnix
		}
		if active[i].Kind != active[j].Kind {
			return active[i].Kind < active[j].Kind
		}
		return active[i].Key < active[j].Key
	})
	return active[:maxContinuationStatuses]
}

func continuationStatusIndex(
	statuses []routingentity.ContinuationStatus,
	kind, key string,
) int {
	for index, status := range statuses {
		if status.Kind == kind && status.Key == key {
			return index
		}
	}
	return -1
}

func (store *Store) continuationStatusPath() string {
	return filepath.Join(store.root, "continuation-status.json")
}
