package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const (
	runtimeJournalMaxTombstones = 16_384
	runtimeJournalMaxPathBytes  = 4 << 10
)

func normalizeJournalEnvelope(value *journalEnvelope) error {
	if value == nil || value.SavedAt != 0 {
		return nil
	}
	object, ok := jsonObject(value.Value)
	if !ok || (object["saved_at"] == nil && object["continuations"] == nil) {
		return nil
	}
	if object["saved_at"] == nil || object["continuations"] == nil {
		return errors.New("continuation journal value is incomplete")
	}
	var savedAt int64
	if json.Unmarshal(object["saved_at"], &savedAt) != nil {
		return errors.New("continuation journal saved_at is invalid")
	}
	value.SavedAt = savedAt
	value.Value = cloneBytes(object["continuations"])
	if raw := object["tombstones"]; raw != nil {
		if err := json.Unmarshal(raw, &value.Tombstones); err != nil {
			return errors.New("continuation journal tombstones are invalid")
		}
	}
	return nil
}

// mergeJournalJSON keeps release tombstones so an older queued snapshot cannot
// resurrect a continuation after a restart. Tombstones are metadata only; the
// returned value remains the caller's continuation JSON.
func mergeJournalJSON(existing, incoming json.RawMessage, previous []string) (json.RawMessage, []string, error) {
	tombstones, err := journalTombstoneSet(previous)
	if err != nil {
		return nil, nil, err
	}
	merged, err := mergeJournalValue(existing, incoming, "", tombstones)
	if err != nil {
		return nil, nil, err
	}
	result := sortedJournalTombstones(tombstones)
	if len(result) > runtimeJournalMaxTombstones {
		return nil, nil, errors.New("continuation journal tombstones exceed limit")
	}
	return merged, result, nil
}

func mergeJournalValue(existing, incoming json.RawMessage, path string, tombstones map[string]struct{}) (json.RawMessage, error) {
	if path != "" {
		if _, blocked := tombstones[path]; blocked {
			return cloneBytes(existing), nil
		}
	}
	if incomingObject, ok := jsonObject(incoming); ok {
		return mergeJournalObject(existing, incomingObject, path, tombstones)
	}
	return cloneBytes(incoming), nil
}

func mergeJournalObject(existing json.RawMessage, incoming map[string]json.RawMessage, path string, tombstones map[string]struct{}) (json.RawMessage, error) {
	left := make(map[string]json.RawMessage)
	if current, ok := jsonObject(existing); ok {
		for key, value := range current {
			left[key] = cloneBytes(value)
		}
	}
	for key, value := range incoming {
		childPath := journalPath(path, key)
		if len(childPath) > runtimeJournalMaxPathBytes {
			return nil, errors.New("continuation journal tombstone path is too long")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(left, key)
			// Record the release even when the key is already absent. An older
			// queued snapshot may still carry that key and must not resurrect it.
			tombstones[childPath] = struct{}{}
			continue
		}
		if _, blocked := tombstones[childPath]; blocked {
			continue
		}
		merged, err := mergeJournalValue(left[key], value, childPath, tombstones)
		if err != nil {
			return nil, err
		}
		left[key] = merged
	}
	return json.Marshal(left)
}

func jsonObject(value json.RawMessage) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if len(value) == 0 || json.Unmarshal(value, &object) != nil || object == nil {
		return nil, false
	}
	return object, true
}

func journalTombstoneSet(values []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(values))
	for _, path := range values {
		if path == "" || len(path) > runtimeJournalMaxPathBytes || !strings.HasPrefix(path, "/") {
			return nil, errors.New("invalid continuation journal tombstone")
		}
		set[path] = struct{}{}
	}
	if len(set) > runtimeJournalMaxTombstones {
		return nil, errors.New("continuation journal tombstones exceed limit")
	}
	return set, nil
}

func sortedJournalTombstones(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for path := range values {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func journalPath(parent, key string) string {
	key = strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
	return parent + "/" + key
}
