package routing

import (
	"bytes"
	"context"
	"encoding/hex"
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

const maxPreviousResponseFailureBytes = 2 << 20

type previousResponseFailureSnapshot struct {
	Version  int                                     `json:"version"`
	Failures []routingentity.PreviousResponseFailure `json:"failures"`
}

func (store *Store) LoadPreviousResponseFailures(
	ctx context.Context,
	now time.Time,
) ([]routingentity.PreviousResponseFailure, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare previous response failure store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "previous-response-failures.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock previous response failure store: %w", err)
	}
	defer release()
	failures, err := store.readPreviousResponseFailures()
	if err != nil {
		return nil, fmt.Errorf("read previous response failure store: %w", err)
	}
	cutoff := now.Add(-routingentity.PreviousResponseFailureRetention).Unix()
	active := failures[:0]
	for _, failure := range failures {
		if failure.UpdatedUnix > cutoff {
			active = append(active, failure)
		}
	}
	return active, nil
}

func (store *Store) RecordPreviousResponseFailure(
	ctx context.Context,
	accountID, responseKey, route string,
	now time.Time,
) (routingentity.PreviousResponseFailure, error) {
	failure := routingentity.PreviousResponseFailure{
		AccountID: accountID, ResponseKey: responseKey, Route: route, UpdatedUnix: now.Unix(),
	}
	if err := failure.Validate(); err != nil {
		return routingentity.PreviousResponseFailure{}, err
	}
	if err := ctx.Err(); err != nil {
		return routingentity.PreviousResponseFailure{}, err
	}
	if err := store.prepare(); err != nil {
		return routingentity.PreviousResponseFailure{}, fmt.Errorf("prepare previous response failure store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "previous-response-failures.guard"))
	if err != nil {
		return routingentity.PreviousResponseFailure{}, fmt.Errorf("lock previous response failure store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return routingentity.PreviousResponseFailure{}, err
	}
	failures, err := store.readPreviousResponseFailures()
	if err != nil {
		return routingentity.PreviousResponseFailure{}, fmt.Errorf("read previous response failure store: %w", err)
	}
	index := previousResponseFailureIndex(failures, accountID, responseKey, route)
	if index >= 0 {
		failure = failures[index]
	}
	failure, err = failure.Record(now)
	if err != nil {
		return routingentity.PreviousResponseFailure{}, err
	}
	if index >= 0 {
		failures[index] = failure
	} else {
		failures = append(failures, failure)
	}
	failures = retainPreviousResponseFailures(failures, now)
	if err := store.writePreviousResponseFailures(failures); err != nil {
		return routingentity.PreviousResponseFailure{}, fmt.Errorf("write previous response failure store: %w", err)
	}
	return failure, nil
}

func (store *Store) ClearPreviousResponseFailures(ctx context.Context, accountID, responseKey string) error {
	if err := routingentity.ValidateAccountID(accountID); err != nil {
		return err
	}
	if !validPreviousResponseKey(responseKey) {
		return errors.New("invalid previous response key")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare previous response failure store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "previous-response-failures.guard"))
	if err != nil {
		return fmt.Errorf("lock previous response failure store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	failures, err := store.readPreviousResponseFailures()
	if err != nil {
		return fmt.Errorf("read previous response failure store: %w", err)
	}
	kept := failures[:0]
	for _, failure := range failures {
		if failure.AccountID != accountID || failure.ResponseKey != responseKey {
			kept = append(kept, failure)
		}
	}
	if len(kept) == len(failures) {
		return nil
	}
	if err := store.writePreviousResponseFailures(kept); err != nil {
		return fmt.Errorf("write previous response failure store: %w", err)
	}
	return nil
}

func (store *Store) readPreviousResponseFailures() ([]routingentity.PreviousResponseFailure, error) {
	path := filepath.Join(store.root, "previous-response-failures.json")
	return readRoutingSnapshot(path, maxPreviousResponseFailureBytes, decodePreviousResponseFailures)
}

func decodePreviousResponseFailures(content []byte) ([]routingentity.PreviousResponseFailure, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var snapshot previousResponseFailureSnapshot
	if decoder.Decode(&snapshot) != nil {
		return nil, errors.New("decode previous response failure snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("previous response failure snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Failures) > routingentity.MaxPreviousResponseFailures {
		return nil, errors.New("unsupported or oversized previous response failure snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Failures))
	for _, failure := range snapshot.Failures {
		if err := failure.Validate(); err != nil {
			return nil, err
		}
		key := previousResponseFailureIdentity(failure.AccountID, failure.ResponseKey, failure.Route)
		if seen[key] {
			return nil, errors.New("previous response failure snapshot has duplicate entries")
		}
		seen[key] = true
	}
	return snapshot.Failures, nil
}

func (store *Store) writePreviousResponseFailures(failures []routingentity.PreviousResponseFailure) error {
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].AccountID != failures[j].AccountID {
			return failures[i].AccountID < failures[j].AccountID
		}
		if failures[i].Route != failures[j].Route {
			return failures[i].Route < failures[j].Route
		}
		return failures[i].ResponseKey < failures[j].ResponseKey
	})
	content, err := json.Marshal(previousResponseFailureSnapshot{Version: 1, Failures: failures})
	if err != nil {
		return err
	}
	if len(content) > maxPreviousResponseFailureBytes {
		return errors.New("previous response failure snapshot exceeds size limit")
	}
	path := filepath.Join(store.root, "previous-response-failures.json")
	return writeRoutingSnapshot(path, content, maxPreviousResponseFailureBytes, func(content []byte) error {
		_, err := decodePreviousResponseFailures(content)
		return err
	})
}

func retainPreviousResponseFailures(
	failures []routingentity.PreviousResponseFailure,
	now time.Time,
) []routingentity.PreviousResponseFailure {
	cutoff := now.Add(-routingentity.PreviousResponseFailureRetention).Unix()
	active := failures[:0]
	for _, failure := range failures {
		if failure.UpdatedUnix > cutoff {
			active = append(active, failure)
		}
	}
	if len(active) > routingentity.MaxPreviousResponseFailures {
		sort.Slice(active, func(i, j int) bool {
			if active[i].UpdatedUnix != active[j].UpdatedUnix {
				return active[i].UpdatedUnix > active[j].UpdatedUnix
			}
			return previousResponseFailureIdentity(active[i].AccountID, active[i].ResponseKey, active[i].Route) <
				previousResponseFailureIdentity(active[j].AccountID, active[j].ResponseKey, active[j].Route)
		})
		active = active[:routingentity.MaxPreviousResponseFailures]
	}
	return active
}

func previousResponseFailureIndex(
	failures []routingentity.PreviousResponseFailure,
	accountID, responseKey, route string,
) int {
	for index, failure := range failures {
		if failure.AccountID == accountID && failure.ResponseKey == responseKey && failure.Route == route {
			return index
		}
	}
	return -1
}

func previousResponseFailureIdentity(accountID, responseKey, route string) string {
	return accountID + ":" + route + ":" + responseKey
}

func validPreviousResponseKey(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
