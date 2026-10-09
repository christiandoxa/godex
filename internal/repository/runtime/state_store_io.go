package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

func readStateEnvelope(path, backup string) (stateSnapshotEnvelope, bool, error) {
	var value stateSnapshotEnvelope
	recovered, err := readVersioned(path, backup, &value)
	if err != nil {
		return value, false, err
	}
	if value.Version == 0 {
		value.Version = runtimeStateVersion
	}
	return value, recovered, nil
}

func readContinuationEnvelope(path, backup string) (continuationEnvelope, bool, error) {
	var value continuationEnvelope
	recovered, err := readVersioned(path, backup, &value)
	if err != nil {
		return value, false, err
	}
	if value.Version == 0 {
		value.Version = runtimeStateVersion
	}
	return value, recovered, nil
}

func readJournalEnvelope(path, backup string) (journalEnvelope, bool, error) {
	var value journalEnvelope
	recovered, err := readVersioned(path, backup, &value)
	if err != nil {
		return value, false, err
	}
	if err := normalizeJournalEnvelope(&value); err != nil {
		return value, false, err
	}
	if value.Version == 0 {
		value.Version = runtimeStateVersion
	}
	if _, err := journalTombstoneSet(value.Tombstones); err != nil {
		return value, false, err
	}
	return value, recovered, nil
}

func readVersioned(path, backup string, target any) (bool, error) {
	primary, primaryErr := readJSON(path)
	if primaryErr == nil {
		if err := decodeVersioned(primary, target); err == nil {
			return false, nil
		} else {
			primaryErr = err
		}
	}
	if errors.Is(primaryErr, os.ErrNotExist) && !pathExists(backup) {
		return false, nil
	}
	backupBytes, err := readJSON(backup)
	if err != nil {
		return false, fmt.Errorf("read runtime state: %w (primary: %v)", err, primaryErr)
	}
	if err := decodeVersioned(backupBytes, target); err != nil {
		return false, fmt.Errorf("decode runtime state backup: %w", err)
	}
	_ = writePrivateJSON(path, backupBytes)
	return true, nil
}

func decodeVersioned(content []byte, target any) error {
	switch value := target.(type) {
	case *stateSnapshotEnvelope:
		*value = stateSnapshotEnvelope{}
	case *continuationEnvelope:
		*value = continuationEnvelope{}
	case *journalEnvelope:
		*value = journalEnvelope{}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(content, &fields) != nil {
		fields = nil
	}
	versioned := fields["generation"] != nil
	if _, journal := target.(*journalEnvelope); journal {
		versioned = versioned || fields["value"] != nil
	}
	if !versioned {
		if !json.Valid(content) {
			return errors.New("decode runtime state: invalid JSON")
		}
		switch value := target.(type) {
		case *stateSnapshotEnvelope:
			value.State = cloneBytes(content)
		case *continuationEnvelope:
			value.Continuations = cloneBytes(content)
		case *journalEnvelope:
			value.Value = cloneBytes(content)
			if err := normalizeJournalEnvelope(value); err != nil {
				return err
			}
			if _, err := journalTombstoneSet(value.Tombstones); err != nil {
				return err
			}
		default:
			return errors.New("unsupported runtime state envelope")
		}
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode runtime state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("runtime state contains trailing JSON")
	}
	switch value := target.(type) {
	case *stateSnapshotEnvelope:
		if value.Version == 0 {
			value.Version = runtimeStateVersion
		}
		if value.Version != runtimeStateVersion {
			return fmt.Errorf("unsupported runtime state version %d", value.Version)
		}
	case *continuationEnvelope:
		if value.Version == 0 {
			value.Version = runtimeStateVersion
		}
		if value.Version != runtimeStateVersion {
			return fmt.Errorf("unsupported runtime continuation version %d", value.Version)
		}
	case *journalEnvelope:
		if value.Version == 0 {
			value.Version = runtimeStateVersion
		}
		if value.Version != runtimeStateVersion {
			return fmt.Errorf("unsupported continuation journal version %d", value.Version)
		}
		if err := normalizeJournalEnvelope(value); err != nil {
			return err
		}
		if _, err := journalTombstoneSet(value.Tombstones); err != nil {
			return err
		}
	}
	return nil
}

func writeEnvelope(path, backup string, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(content) > runtimeStateMaxBytes {
		return errors.New("runtime state exceeds size limit")
	}
	if _, err := fileutil.AtomicWrite(path, content); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(backup, content); err != nil {
		return err
	}
	return nil
}

func writePrivateJSON(path string, content []byte) error {
	if len(content) > runtimeStateMaxBytes {
		return errors.New("runtime state exceeds size limit")
	}
	_, err := fileutil.AtomicWrite(path, content)
	return err
}

func readJSON(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("runtime state path must be a regular file")
	}
	if info.Size() > runtimeStateMaxBytes {
		return nil, errors.New("runtime state exceeds size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, opened) || !os.SameFile(info, current) {
		return nil, errors.New("runtime state path changed while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, runtimeStateMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > runtimeStateMaxBytes {
		return nil, errors.New("runtime state exceeds size limit")
	}
	return content, nil
}

func validRaw(value []byte) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage("null"), nil
	}
	if len(value) > runtimeStateMaxBytes || !json.Valid(value) {
		return nil, errors.New("invalid JSON")
	}
	return append(json.RawMessage(nil), value...), nil
}

func mergeJSON(existing, incoming json.RawMessage) (json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(incoming), []byte("null")) {
		return append(json.RawMessage(nil), incoming...), nil
	}
	if len(existing) == 0 || bytes.Equal(bytes.TrimSpace(existing), []byte("null")) {
		return append(json.RawMessage(nil), incoming...), nil
	}
	var left, right map[string]json.RawMessage
	if json.Unmarshal(existing, &left) != nil || json.Unmarshal(incoming, &right) != nil {
		return append(json.RawMessage(nil), incoming...), nil
	}
	for key, value := range right {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(left, key)
			continue
		}
		if current, ok := left[key]; ok {
			merged, err := mergeJSON(current, value)
			if err != nil {
				return nil, err
			}
			left[key] = merged
		} else {
			left[key] = append(json.RawMessage(nil), value...)
		}
	}
	return json.Marshal(left)
}

func cloneBytes(value []byte) []byte { return append([]byte(nil), value...) }

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func consumeFault(names ...string) error {
	for _, name := range names {
		if os.Getenv(name) == "" {
			continue
		}
		_ = os.Unsetenv(name)
		return errors.New("injected runtime state save failure")
	}
	return nil
}
