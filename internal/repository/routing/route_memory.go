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
	maxRouteMemoryScores = 3072
	maxRouteMemoryBytes  = 2 << 20
)

type routeMemorySnapshot struct {
	Version int                              `json:"version"`
	Scores  []routingentity.RouteMemoryScore `json:"scores"`
}

func (store *Store) LoadRouteMemory(ctx context.Context, now time.Time) ([]routingentity.RouteMemoryScore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing memory store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-memory.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing memory store: %w", err)
	}
	defer release()
	scores, err := store.readRouteMemory()
	if err != nil {
		return nil, err
	}
	active := scores[:0]
	cutoff := now.Add(-routingentity.RouteMemoryRetention).Unix()
	for _, score := range scores {
		if score.UpdatedUnix > cutoff && score.Effective(now) > 0 {
			active = append(active, score)
		}
	}
	return active, nil
}

func (store *Store) MutateRouteMemory(ctx context.Context, accountID, route, kind string, now time.Time, mutation func(routingentity.RouteMemoryScore) routingentity.RouteMemoryScore) (routingentity.RouteMemoryScore, error) {
	if err := ctx.Err(); err != nil {
		return routingentity.RouteMemoryScore{}, err
	}
	if mutation == nil {
		return routingentity.RouteMemoryScore{}, errors.New("routing memory mutation is required")
	}
	if err := store.prepare(); err != nil {
		return routingentity.RouteMemoryScore{}, fmt.Errorf("prepare routing memory store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-memory.guard"))
	if err != nil {
		return routingentity.RouteMemoryScore{}, fmt.Errorf("lock routing memory store: %w", err)
	}
	defer release()
	scores, err := store.readRouteMemory()
	if err != nil {
		return routingentity.RouteMemoryScore{}, err
	}
	current := routingentity.RouteMemoryScore{AccountID: accountID, Route: route, Kind: kind, UpdatedUnix: now.Unix()}
	index := -1
	for candidate, score := range scores {
		if score.AccountID == accountID && score.Route == route && score.Kind == kind {
			current, index = score, candidate
			break
		}
	}
	updated := mutation(current)
	updated.AccountID, updated.Route, updated.Kind = accountID, route, kind
	if updated.UpdatedUnix < current.UpdatedUnix {
		updated.UpdatedUnix = current.UpdatedUnix
	}
	if updated.UpdatedUnix < now.Unix() {
		updated.UpdatedUnix = now.Unix()
	}
	if err := updated.Validate(); err != nil {
		return routingentity.RouteMemoryScore{}, err
	}
	if updated.Score == 0 {
		if index >= 0 {
			scores = append(scores[:index], scores[index+1:]...)
		}
	} else if index >= 0 {
		scores[index] = updated
	} else {
		scores = append(scores, updated)
	}
	scores = retainRouteMemory(scores, now)
	if err := store.writeRouteMemory(scores); err != nil {
		return routingentity.RouteMemoryScore{}, err
	}
	return updated, nil
}

func (store *Store) readRouteMemory() ([]routingentity.RouteMemoryScore, error) {
	path := filepath.Join(store.root, "route-memory.json")
	return readRoutingSnapshot(path, maxRouteMemoryBytes, decodeRouteMemory)
}

func decodeRouteMemory(content []byte) ([]routingentity.RouteMemoryScore, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var snapshot routeMemorySnapshot
	if decoder.Decode(&snapshot) != nil {
		return nil, errors.New("decode routing memory snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("routing memory snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Scores) > maxRouteMemoryScores {
		return nil, errors.New("unsupported or oversized routing memory snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Scores))
	for _, score := range snapshot.Scores {
		if err := score.Validate(); err != nil {
			return nil, err
		}
		key := score.AccountID + ":" + score.Route + ":" + score.Kind
		if seen[key] {
			return nil, errors.New("routing memory snapshot has duplicate scores")
		}
		seen[key] = true
	}
	return snapshot.Scores, nil
}

func (store *Store) writeRouteMemory(scores []routingentity.RouteMemoryScore) error {
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].AccountID != scores[j].AccountID {
			return scores[i].AccountID < scores[j].AccountID
		}
		if scores[i].Route != scores[j].Route {
			return scores[i].Route < scores[j].Route
		}
		return scores[i].Kind < scores[j].Kind
	})
	content, err := json.Marshal(routeMemorySnapshot{Version: 1, Scores: scores})
	if err != nil {
		return err
	}
	path := filepath.Join(store.root, "route-memory.json")
	return writeRoutingSnapshot(path, content, maxRouteMemoryBytes, func(content []byte) error {
		_, err := decodeRouteMemory(content)
		return err
	})
}

func retainRouteMemory(scores []routingentity.RouteMemoryScore, now time.Time) []routingentity.RouteMemoryScore {
	cutoff := now.Add(-routingentity.RouteMemoryRetention).Unix()
	active := scores[:0]
	for _, score := range scores {
		if score.UpdatedUnix > cutoff {
			active = append(active, score)
		}
	}
	if len(active) > maxRouteMemoryScores {
		sort.Slice(active, func(i, j int) bool { return active[i].UpdatedUnix > active[j].UpdatedUnix })
		active = active[:maxRouteMemoryScores]
	}
	return active
}
