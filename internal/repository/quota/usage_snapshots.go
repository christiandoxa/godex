package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	usageSnapshotsFileName   = "runtime-usage-snapshots.json"
	usageSnapshotsMaxBytes   = 2 << 20
	usageSnapshotsMaxEntries = 4096
)

type UsageSnapshotStore struct{ root string }

type usageSnapshotEnvelope struct {
	Generation uint64                              `json:"generation"`
	Value      map[string]quotamodel.UsageSnapshot `json:"value"`
}

func NewUsageSnapshotStore(root string) *UsageSnapshotStore {
	return &UsageSnapshotStore{root: filepath.Clean(root)}
}

func (store *UsageSnapshotStore) Load(ctx context.Context, accountID string) (quotamodel.UsageSnapshot, bool, error) {
	if err := ctx.Err(); err != nil {
		return quotamodel.UsageSnapshot{}, false, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return quotamodel.UsageSnapshot{}, false, nil
	}
	release, err := store.lock(ctx)
	if err != nil {
		return quotamodel.UsageSnapshot{}, false, err
	}
	defer release()
	envelope, err := store.readRecovering()
	if err != nil {
		return quotamodel.UsageSnapshot{}, false, err
	}
	snapshot, ok := envelope.Value[accountID]
	return snapshot, ok, nil
}

func (store *UsageSnapshotStore) Save(ctx context.Context, accountID string, snapshot quotamodel.UsageSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("quota usage snapshot account ID is required")
	}
	if err := validateUsageSnapshot(snapshot); err != nil {
		return err
	}
	release, err := store.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	envelope, err := store.readRecovering()
	if err != nil {
		return err
	}
	if envelope.Value == nil {
		envelope.Value = make(map[string]quotamodel.UsageSnapshot)
	}
	if current, ok := envelope.Value[accountID]; !ok || snapshot.CheckedAt >= current.CheckedAt {
		envelope.Value[accountID] = snapshot
	}
	if len(envelope.Value) > usageSnapshotsMaxEntries {
		type entry struct {
			id string
			at int64
		}
		entries := make([]entry, 0, len(envelope.Value))
		for id, value := range envelope.Value {
			entries = append(entries, entry{id: id, at: value.CheckedAt})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].at != entries[j].at {
				return entries[i].at > entries[j].at
			}
			return entries[i].id < entries[j].id
		})
		keep := make(map[string]quotamodel.UsageSnapshot, usageSnapshotsMaxEntries)
		for _, item := range entries[:usageSnapshotsMaxEntries] {
			keep[item.id] = envelope.Value[item.id]
		}
		envelope.Value = keep
	}
	envelope.Generation++
	return store.write(envelope)
}

func (store *UsageSnapshotStore) lock(ctx context.Context) (func() error, error) {
	if store == nil || strings.TrimSpace(store.root) == "" {
		return nil, errors.New("quota usage snapshot root is required")
	}
	info, err := os.Lstat(store.root)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(store.root, 0o700); err != nil {
			return nil, fmt.Errorf("prepare quota usage snapshot root: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("inspect quota usage snapshot root: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return nil, errors.New("quota usage snapshot root must be a real directory")
	default:
		if err := os.Chmod(store.root, 0o700); err != nil {
			return nil, fmt.Errorf("secure quota usage snapshot root: %w", err)
		}
	}
	return lockfile.Acquire(ctx, filepath.Join(store.root, "runtime-usage-snapshots.guard"))
}

func (store *UsageSnapshotStore) readRecovering() (usageSnapshotEnvelope, error) {
	primary := store.path()
	backup := store.backupPath()
	if envelope, err := readUsageSnapshotEnvelope(primary); err == nil {
		return envelope, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		if recovered, backupErr := readUsageSnapshotEnvelope(backup); backupErr == nil {
			content, marshalErr := json.Marshal(recovered)
			if marshalErr == nil {
				_, _ = fileutil.AtomicWrite(primary, content)
			}
			return recovered, nil
		}
		return usageSnapshotEnvelope{}, err
	}
	if envelope, err := readUsageSnapshotEnvelope(backup); err == nil {
		content, marshalErr := json.Marshal(envelope)
		if marshalErr == nil {
			_, _ = fileutil.AtomicWrite(primary, content)
		}
		return envelope, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return usageSnapshotEnvelope{}, err
	}
	return usageSnapshotEnvelope{Value: make(map[string]quotamodel.UsageSnapshot)}, nil
}

func readUsageSnapshotEnvelope(path string) (usageSnapshotEnvelope, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return usageSnapshotEnvelope{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > usageSnapshotsMaxBytes {
		return usageSnapshotEnvelope{}, errors.New("quota usage snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return usageSnapshotEnvelope{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, usageSnapshotsMaxBytes+1))
	decoder.DisallowUnknownFields()
	var envelope usageSnapshotEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return usageSnapshotEnvelope{}, errors.New("decode quota usage snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return usageSnapshotEnvelope{}, errors.New("quota usage snapshot has trailing data")
	}
	if len(envelope.Value) > usageSnapshotsMaxEntries {
		return usageSnapshotEnvelope{}, errors.New("quota usage snapshot exceeds entry limit")
	}
	for id, snapshot := range envelope.Value {
		if strings.TrimSpace(id) == "" || len(id) > 4096 {
			return usageSnapshotEnvelope{}, errors.New("quota usage snapshot has invalid account ID")
		}
		if err := validateUsageSnapshot(snapshot); err != nil {
			return usageSnapshotEnvelope{}, err
		}
	}
	return envelope, nil
}

func validateUsageSnapshot(snapshot quotamodel.UsageSnapshot) error {
	if snapshot.CheckedAt <= 0 {
		return errors.New("quota usage snapshot checked_at is invalid")
	}
	for _, status := range []quotamodel.WindowStatus{snapshot.FiveHourStatus, snapshot.WeeklyStatus} {
		switch status {
		case quotamodel.WindowReady, quotamodel.WindowThin, quotamodel.WindowCritical, quotamodel.WindowExhausted, quotamodel.WindowUnknown:
		default:
			return errors.New("quota usage snapshot has invalid window status")
		}
	}
	for _, remaining := range []int64{snapshot.FiveHourRemainingPercent, snapshot.WeeklyRemainingPercent} {
		if remaining < 0 || remaining > 100 {
			return errors.New("quota usage snapshot has invalid remaining percent")
		}
	}
	return nil
}

func (store *UsageSnapshotStore) write(envelope usageSnapshotEnvelope) error {
	content, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(content) > usageSnapshotsMaxBytes {
		return errors.New("quota usage snapshot exceeds size limit")
	}
	if _, err := fileutil.AtomicWrite(store.path(), content); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(store.backupPath(), content); err != nil {
		return err
	}
	return nil
}

func (store *UsageSnapshotStore) path() string {
	return filepath.Join(store.root, usageSnapshotsFileName)
}
func (store *UsageSnapshotStore) backupPath() string { return store.path() + ".last-good" }
