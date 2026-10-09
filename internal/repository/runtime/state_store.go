package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	runtimeStateFileName           = "runtime-state.json"
	runtimeContinuationsFileName   = "runtime-continuations.json"
	runtimeContinuationJournalName = "runtime-continuation-journal.json"
	runtimeLastGoodSuffix          = ".last-good"
	runtimeStateVersion            = 1
	runtimeStateMaxBytes           = 4 << 20
	runtimeStateLockName           = "runtime-state.lock"
	runtimeStateSectionsMask       = 0x3b
	runtimeStateFaultEnv           = "GODEX_RUNTIME_FAULT_STATE_SAVE_ERROR_ONCE"
	runtimeContinuationFaultEnv    = "GODEX_RUNTIME_FAULT_CONTINUATION_JOURNAL_SAVE_ERROR_ONCE"
	legacyRuntimeStateFaultEnv     = "PRODEX_RUNTIME_FAULT_STATE_SAVE_ERROR_ONCE"
	legacyContinuationFaultEnv     = "PRODEX_RUNTIME_FAULT_CONTINUATIONS_SAVE_ERROR_ONCE"
)

// StateStore persists runtime snapshots and their continuation sidecars.
// Every file has a private primary and a last-good copy; a corrupt primary is
// repaired from the last-good copy before the value is returned.
type StateStore struct{ root string }

// RecoveredSnapshot reports whether Load used a last-good file.
type RecoveredSnapshot struct {
	Snapshot
	RecoveredFromBackup bool
	Generation          uint64
}

// RecoveredJournal is the continuation-journal equivalent of RecoveredSnapshot.
type RecoveredJournal struct {
	Data                []byte
	SavedAt             int64
	RecoveredFromBackup bool
	Generation          uint64
}

type stateSnapshotEnvelope struct {
	Version        int             `json:"version"`
	Generation     uint64          `json:"generation"`
	State          json.RawMessage `json:"state,omitempty"`
	ProfileScores  json.RawMessage `json:"profile_scores,omitempty"`
	UsageSnapshots json.RawMessage `json:"usage_snapshots,omitempty"`
	Backoffs       json.RawMessage `json:"backoffs,omitempty"`
}

type continuationEnvelope struct {
	Version       int             `json:"version"`
	Generation    uint64          `json:"generation"`
	Continuations json.RawMessage `json:"continuations,omitempty"`
}

type journalEnvelope struct {
	Version    int             `json:"version"`
	Generation uint64          `json:"generation"`
	SavedAt    int64           `json:"saved_at"`
	Value      json.RawMessage `json:"value,omitempty"`
	Tombstones []string        `json:"tombstones,omitempty"`
}

// NewStateStore creates a runtime state store rooted at root.
func NewStateStore(root string) *StateStore { return &StateStore{root: filepath.Clean(root)} }

// Root returns the configured state root. It is useful to expose paths to
// diagnostics without exposing the store's mutable internals.
func (store *StateStore) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

func (store *StateStore) StatePath() string { return filepath.Join(store.root, runtimeStateFileName) }

func (store *StateStore) StateLastGoodPath() string {
	return store.StatePath() + runtimeLastGoodSuffix
}

func (store *StateStore) ContinuationsPath() string {
	return filepath.Join(store.root, runtimeContinuationsFileName)
}

func (store *StateStore) ContinuationsLastGoodPath() string {
	return store.ContinuationsPath() + runtimeLastGoodSuffix
}

func (store *StateStore) ContinuationJournalPath() string {
	return filepath.Join(store.root, runtimeContinuationJournalName)
}

func (store *StateStore) ContinuationJournalLastGoodPath() string {
	return store.ContinuationJournalPath() + runtimeLastGoodSuffix
}

// Load returns the latest recoverable runtime snapshot. Missing files are an
// empty snapshot, which keeps first launch equivalent to a fresh process.
func (store *StateStore) Load(ctx context.Context) (Snapshot, error) {
	loaded, err := store.LoadWithRecovery(ctx)
	return loaded.Snapshot, err
}

// LoadWithRecovery reads the state and continuation sidecars under one lock.
func (store *StateStore) LoadWithRecovery(ctx context.Context) (RecoveredSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return RecoveredSnapshot{}, err
	}
	if err := store.prepare(); err != nil {
		return RecoveredSnapshot{}, err
	}
	release, err := store.lock(ctx)
	if err != nil {
		return RecoveredSnapshot{}, err
	}
	defer release()

	state, stateRecovered, err := readStateEnvelope(store.StatePath(), store.StateLastGoodPath())
	if err != nil {
		return RecoveredSnapshot{}, err
	}
	continuations, continuationRecovered, err := readContinuationEnvelope(
		store.ContinuationsPath(), store.ContinuationsLastGoodPath(),
	)
	if err != nil {
		return RecoveredSnapshot{}, err
	}
	return RecoveredSnapshot{
		Snapshot: Snapshot{
			State:          cloneBytes(state.State),
			Continuations:  cloneBytes(continuations.Continuations),
			ProfileScores:  cloneBytes(state.ProfileScores),
			UsageSnapshots: cloneBytes(state.UsageSnapshots),
			Backoffs:       cloneBytes(state.Backoffs),
		},
		RecoveredFromBackup: stateRecovered || continuationRecovered,
		Generation:          maxUint64(state.Generation, continuations.Generation),
	}, nil
}

// LoadContinuationJournal loads the journal independently of the state file.
func (store *StateStore) LoadContinuationJournal(ctx context.Context) (RecoveredJournal, error) {
	if err := ctx.Err(); err != nil {
		return RecoveredJournal{}, err
	}
	if err := store.prepare(); err != nil {
		return RecoveredJournal{}, err
	}
	release, err := store.lock(ctx)
	if err != nil {
		return RecoveredJournal{}, err
	}
	defer release()
	envelope, recovered, err := readJournalEnvelope(
		store.ContinuationJournalPath(), store.ContinuationJournalLastGoodPath(),
	)
	if err != nil {
		return RecoveredJournal{}, err
	}
	return RecoveredJournal{
		Data:                cloneBytes(envelope.Value),
		SavedAt:             envelope.SavedAt,
		RecoveredFromBackup: recovered,
		Generation:          envelope.Generation,
	}, nil
}

// Save writes every supplied section. The continuation sidecar is committed
// before the state sidecar, matching the stronger continuation ordering used by
// Prodex when a process is interrupted between writes.
func (store *StateStore) Save(
	ctx context.Context,
	state, continuations, profileScores, usageSnapshots, backoffs []byte,
) error {
	var mask uint8
	if state != nil {
		mask |= runtimeStateFullMask
	}
	if continuations != nil {
		mask |= runtimeContinuationsMask
	}
	if profileScores != nil {
		mask |= runtimeProfileScoresMask
	}
	if usageSnapshots != nil {
		mask |= runtimeUsageSnapshotsMask
	}
	if backoffs != nil {
		mask |= runtimeBackoffsMask
	}
	return store.SaveSelected(ctx, state, continuations, profileScores, usageSnapshots, backoffs, mask)
}

// SaveSelected applies only the sections named by mask and preserves every
// unselected section already on disk. Mask bits are state=0x03 (core=0x01,
// full=0x02), continuations=0x04, profile scores=0x08, usage snapshots=0x10,
// and backoffs=0x20.
func (store *StateStore) SaveSelected(
	ctx context.Context,
	state, continuations, profileScores, usageSnapshots, backoffs []byte,
	mask uint8,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if mask == 0 {
		return nil
	}
	if mask&^uint8(runtimeSaveMask) != 0 || mask&runtimeStateMask == runtimeStateMask {
		return errors.New("invalid runtime state save section mask")
	}
	for _, section := range []struct {
		bit  uint8
		data []byte
		name string
	}{
		{runtimeStateMask, state, "runtime state"},
		{runtimeContinuationsMask, continuations, "runtime continuations"},
		{runtimeProfileScoresMask, profileScores, "runtime profile scores"},
		{runtimeUsageSnapshotsMask, usageSnapshots, "runtime usage snapshots"},
		{runtimeBackoffsMask, backoffs, "runtime backoffs"},
	} {
		if mask&section.bit != 0 && section.data == nil {
			return fmt.Errorf("%s section is required", section.name)
		}
	}
	if err := store.prepare(); err != nil {
		return err
	}
	release, err := store.lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	currentState := stateSnapshotEnvelope{Version: runtimeStateVersion}
	if mask&runtimeStateEnvelopeMask != 0 {
		currentState, _, err = readStateEnvelope(store.StatePath(), store.StateLastGoodPath())
		if err != nil {
			return err
		}
	}
	currentContinuations := continuationEnvelope{Version: runtimeStateVersion}
	if mask&0x04 != 0 {
		currentContinuations, _, err = readContinuationEnvelope(
			store.ContinuationsPath(), store.ContinuationsLastGoodPath(),
		)
		if err != nil {
			return err
		}
	}
	nextState := currentState
	nextContinuations := currentContinuations
	nextState.Generation++
	nextContinuations.Generation++

	if mask&runtimeStateMask != 0 {
		incoming, err := validRaw(state)
		if err != nil {
			return fmt.Errorf("validate runtime state: %w", err)
		}
		if mask&runtimeStateFullMask != 0 {
			nextState.State = incoming
		} else {
			nextState.State, err = mergeJSON(nextState.State, incoming)
			if err != nil {
				return fmt.Errorf("merge runtime state: %w", err)
			}
		}
	}
	if mask&runtimeContinuationsMask != 0 {
		incoming, err := validRaw(continuations)
		if err != nil {
			return fmt.Errorf("validate runtime continuations: %w", err)
		}
		nextContinuations.Continuations, err = mergeJSON(nextContinuations.Continuations, incoming)
		if err != nil {
			return fmt.Errorf("merge runtime continuations: %w", err)
		}
	}
	for _, section := range []struct {
		bit    uint8
		value  []byte
		target *json.RawMessage
	}{
		{runtimeProfileScoresMask, profileScores, &nextState.ProfileScores},
		{runtimeUsageSnapshotsMask, usageSnapshots, &nextState.UsageSnapshots},
		{runtimeBackoffsMask, backoffs, &nextState.Backoffs},
	} {
		if mask&section.bit == 0 {
			continue
		}
		incoming, err := validRaw(section.value)
		if err != nil {
			return fmt.Errorf("validate runtime sidecar: %w", err)
		}
		*section.target, err = mergeJSON(*section.target, incoming)
		if err != nil {
			return fmt.Errorf("merge runtime sidecar: %w", err)
		}
	}

	if mask&runtimeContinuationsMask != 0 {
		if err := consumeFault(runtimeContinuationFaultEnv, legacyContinuationFaultEnv); err != nil {
			return err
		}
		if err := writeEnvelope(store.ContinuationsPath(), store.ContinuationsLastGoodPath(), nextContinuations); err != nil {
			return fmt.Errorf("write runtime continuations: %w", err)
		}
	}
	if mask&runtimeStateEnvelopeMask != 0 {
		if err := consumeFault(runtimeStateFaultEnv, legacyRuntimeStateFaultEnv); err != nil {
			return err
		}
		if err := writeEnvelope(store.StatePath(), store.StateLastGoodPath(), nextState); err != nil {
			return fmt.Errorf("write runtime state: %w", err)
		}
	}
	return nil
}

// SaveContinuationJournal merges a journal generation and keeps the newest
// saved-at timestamp. JSON object values merge recursively; null removes a key.
func (store *StateStore) SaveContinuationJournal(ctx context.Context, data []byte, savedAt int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return err
	}
	release, err := store.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	incoming, err := validRaw(data)
	if err != nil {
		return fmt.Errorf("validate continuation journal: %w", err)
	}
	current, _, err := readJournalEnvelope(store.ContinuationJournalPath(), store.ContinuationJournalLastGoodPath())
	if err != nil {
		return err
	}
	value, tombstones, err := mergeJournalJSON(current.Value, incoming, current.Tombstones)
	if err != nil {
		return fmt.Errorf("merge continuation journal: %w", err)
	}
	if savedAt < current.SavedAt {
		savedAt = current.SavedAt
	}
	next := current
	next.Version = runtimeStateVersion
	next.Generation++
	next.SavedAt = savedAt
	next.Value = value
	next.Tombstones = tombstones
	if err := consumeFault(runtimeContinuationFaultEnv, legacyContinuationFaultEnv); err != nil {
		return err
	}
	if err := writeEnvelope(store.ContinuationJournalPath(), store.ContinuationJournalLastGoodPath(), next); err != nil {
		return fmt.Errorf("write continuation journal: %w", err)
	}
	return nil
}

func (store *StateStore) prepare() error {
	if store == nil || !filepath.IsAbs(store.root) || store.root == filepath.Dir(store.root) {
		return errors.New("invalid runtime state root")
	}
	info, err := os.Lstat(store.root)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(store.root, 0o700); err != nil {
			return fmt.Errorf("prepare runtime state root: %w", err)
		}
		info, err = os.Lstat(store.root)
	}
	if err != nil {
		return fmt.Errorf("inspect runtime state root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime state root must be a real directory")
	}
	return os.Chmod(store.root, 0o700)
}

func (store *StateStore) lock(ctx context.Context) (func() error, error) {
	return lockfile.Acquire(ctx, filepath.Join(store.root, runtimeStateLockName))
}
