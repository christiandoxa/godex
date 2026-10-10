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
	maxTransportBackoffs     = 4096
	maxTransportBackoffBytes = 1 << 20
)

type transportBackoffSnapshot struct {
	Version  int                              `json:"version"`
	Backoffs []routingentity.TransportBackoff `json:"backoffs"`
}

func (store *Store) LoadTransportBackoffs(ctx context.Context, now time.Time) ([]routingentity.TransportBackoff, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing transport backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-transport-backoff.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing transport backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := store.readTransportBackoffs()
	if err != nil {
		return nil, fmt.Errorf("read routing transport backoff store: %w", err)
	}
	minimumUntil := now.Add(routingentity.InitialTransportBackoffDuration).Unix()
	active := snapshot.Backoffs[:0]
	changed := false
	for _, backoff := range snapshot.Backoffs {
		if backoff.UntilUnix <= now.Unix() {
			changed = true
			continue
		}
		if backoff.UntilUnix > minimumUntil {
			backoff.UntilUnix = minimumUntil
			changed = true
		}
		active = append(active, backoff)
	}
	if changed {
		if err := store.writeTransportBackoffs(active); err != nil {
			return nil, fmt.Errorf("soften routing transport backoffs: %w", err)
		}
	}
	return active, nil
}

func (store *Store) SetTransportBackoff(ctx context.Context, backoff routingentity.TransportBackoff, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := backoff.Validate(); err != nil {
		return err
	}
	if backoff.UntilUnix <= now.Unix() || backoff.UntilUnix > now.Add(routingentity.MaxTransportBackoffDuration).Unix() {
		return errors.New("routing transport backoff exceeds maximum duration")
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing transport backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-transport-backoff.guard"))
	if err != nil {
		return fmt.Errorf("lock routing transport backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := store.readTransportBackoffs()
	if err != nil {
		return fmt.Errorf("read routing transport backoff store: %w", err)
	}
	for index, current := range snapshot.Backoffs {
		if current.AccountID == backoff.AccountID && current.Route == backoff.Route {
			if current.UntilUnix >= backoff.UntilUnix {
				return nil
			}
			snapshot.Backoffs[index] = backoff
			return store.writeTransportBackoffs(retainTransportBackoffs(snapshot.Backoffs, now))
		}
	}
	snapshot.Backoffs = append(snapshot.Backoffs, backoff)
	return store.writeTransportBackoffs(retainTransportBackoffs(snapshot.Backoffs, now))
}

func (store *Store) ClearTransportBackoff(ctx context.Context, accountID, route string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := (routingentity.TransportBackoff{AccountID: accountID, Route: route, UntilUnix: 1}).Validate(); err != nil {
		return err
	}
	if err := store.prepare(); err != nil {
		return fmt.Errorf("prepare routing transport backoff store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-transport-backoff.guard"))
	if err != nil {
		return fmt.Errorf("lock routing transport backoff store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := store.readTransportBackoffs()
	if err != nil {
		return fmt.Errorf("read routing transport backoff store: %w", err)
	}
	remaining := snapshot.Backoffs[:0]
	for _, backoff := range snapshot.Backoffs {
		if backoff.AccountID != accountID || backoff.Route != route {
			remaining = append(remaining, backoff)
		}
	}
	if len(remaining) == len(snapshot.Backoffs) {
		return nil
	}
	return store.writeTransportBackoffs(remaining)
}

func (store *Store) readTransportBackoffs() (transportBackoffSnapshot, error) {
	path := filepath.Join(store.root, "transport-backoff.json")
	snapshot, err := readRoutingSnapshot(path, maxTransportBackoffBytes, decodeTransportBackoffs)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	return snapshot, nil
}

func decodeTransportBackoffs(content []byte) (transportBackoffSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	snapshot := transportBackoffSnapshot{}
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, errors.New("decode routing transport backoff snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return snapshot, errors.New("routing transport backoff snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Backoffs) > maxTransportBackoffs {
		return snapshot, errors.New("unsupported or oversized routing transport backoff snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Backoffs))
	for _, backoff := range snapshot.Backoffs {
		if err := backoff.Validate(); err != nil {
			return snapshot, err
		}
		key := backoff.AccountID + "\x00" + backoff.Route
		if seen[key] {
			return snapshot, errors.New("routing transport backoff snapshot has duplicate routes")
		}
		seen[key] = true
	}
	return snapshot, nil
}

func (store *Store) writeTransportBackoffs(backoffs []routingentity.TransportBackoff) error {
	sort.Slice(backoffs, func(i, j int) bool {
		if backoffs[i].AccountID != backoffs[j].AccountID {
			return backoffs[i].AccountID < backoffs[j].AccountID
		}
		return backoffs[i].Route < backoffs[j].Route
	})
	content, err := json.Marshal(transportBackoffSnapshot{Version: 1, Backoffs: backoffs})
	if err != nil {
		return err
	}
	path := filepath.Join(store.root, "transport-backoff.json")
	return writeRoutingSnapshot(path, content, maxTransportBackoffBytes, func(content []byte) error {
		_, err := decodeTransportBackoffs(content)
		return err
	})
}

func retainTransportBackoffs(backoffs []routingentity.TransportBackoff, now time.Time) []routingentity.TransportBackoff {
	active := backoffs[:0]
	for _, backoff := range backoffs {
		if backoff.UntilUnix > now.Unix() {
			active = append(active, backoff)
		}
	}
	if len(active) > maxTransportBackoffs {
		sort.Slice(active, func(i, j int) bool {
			if active[i].UntilUnix != active[j].UntilUnix {
				return active[i].UntilUnix > active[j].UntilUnix
			}
			if active[i].AccountID != active[j].AccountID {
				return active[i].AccountID < active[j].AccountID
			}
			return active[i].Route < active[j].Route
		})
		active = active[:maxTransportBackoffs]
	}
	return active
}
