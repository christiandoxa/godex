package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const routingSnapshotMaxBytes = 2 << 20

type snapshot struct {
	Version              int                     `json:"version"`
	Bindings             []routingentity.Binding `json:"bindings"`
	SessionReleaseEpochs map[string]int64        `json:"session_release_epochs,omitempty"`
}

func (store *Store) writeState(value snapshot) error {
	value.Version = 1
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(content) > routingSnapshotMaxBytes {
		return errors.New("routing snapshot exceeds size limit")
	}
	if _, err := fileutil.AtomicWrite(store.snapshotPath(), content); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(store.backupPath(), content); err != nil {
		return fmt.Errorf("write routing snapshot backup: %w", err)
	}
	return nil
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
	state, err := store.readState()
	return state.Bindings, err
}

func (store *Store) readState() (snapshot, error) {
	primary, found, primaryErr := readSnapshot(store.snapshotPath())
	if primaryErr == nil && found {
		return primary, nil
	}
	backup, backupFound, backupErr := readSnapshot(store.backupPath())
	if backupErr != nil {
		if primaryErr != nil {
			return snapshot{}, primaryErr
		}
		return snapshot{}, backupErr
	}
	if backupFound {
		content, err := json.Marshal(backup)
		if err == nil {
			// Restore the entire state, including release fences, not just
			// session bindings. Old snapshots must not undo a rebind.
			_, _ = fileutil.AtomicWrite(store.snapshotPath(), content)
		}
		return backup, nil
	}
	if primaryErr != nil {
		return snapshot{}, primaryErr
	}
	return snapshot{Version: 1}, nil
}

func readSnapshot(path string) (snapshot, bool, error) {
	file, found, err := openSnapshot(path)
	if err != nil || !found {
		return snapshot{}, found, err
	}
	defer file.Close()
	value, err := decodeSnapshot(file)
	if err != nil {
		return snapshot{}, true, err
	}
	value.Bindings, err = filterSnapshotBindings(value.Bindings)
	if err != nil {
		return snapshot{}, true, err
	}
	value.Bindings = filterReleasedSessionBindings(value.Bindings, value.SessionReleaseEpochs)
	return value, true, nil
}

func openSnapshot(path string) (*os.File, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > routingSnapshotMaxBytes {
		return nil, false, errors.New("routing snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	return file, err == nil, err
}

func decodeSnapshot(file *os.File) (snapshot, error) {
	decoder := json.NewDecoder(io.LimitReader(file, routingSnapshotMaxBytes+1))
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
	if len(value.Bindings) > routingentity.MaxBindings ||
		len(value.SessionReleaseEpochs) > routingentity.MaxBindings {
		return snapshot{}, errors.New("routing snapshot exceeds binding or fence limit")
	}
	if err := validateSessionReleaseEpochs(value.SessionReleaseEpochs); err != nil {
		return snapshot{}, err
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

func (store *Store) snapshotPath() string {
	return filepath.Join(store.root, "routing.json")
}

func (store *Store) backupPath() string {
	return store.snapshotPath() + ".last-good"
}
