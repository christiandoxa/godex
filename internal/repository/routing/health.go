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
	maxRouteHealthScores = 1024
	maxRouteHealthBytes  = 1 << 20
)

type routeHealthSnapshot struct {
	Version int                              `json:"version"`
	Scores  []routingentity.RouteHealthScore `json:"scores"`
}

func (store *Store) LoadRouteHealth(ctx context.Context, now time.Time) ([]routingentity.RouteHealthScore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.prepare(); err != nil {
		return nil, fmt.Errorf("prepare routing health store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return nil, fmt.Errorf("lock routing health store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scores, err := store.readRouteHealth()
	if err != nil {
		return nil, fmt.Errorf("read routing health store: %w", err)
	}
	cutoff := now.Add(-routingentity.RouteHealthRetention).Unix()
	active := scores[:0]
	for _, score := range scores {
		if score.UpdatedUnix > cutoff && score.Effective(now) > 0 {
			active = append(active, score)
		}
	}
	return active, nil
}

func (store *Store) SetRouteHealth(ctx context.Context, accountID, route string, value uint8, now time.Time) (routingentity.RouteHealthScore, error) {
	if value > routingentity.MaxRouteHealthScore {
		return routingentity.RouteHealthScore{}, errors.New("invalid routing health score")
	}
	if err := ctx.Err(); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	if err := store.prepare(); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	defer release()
	scores, err := store.readRouteHealth()
	if err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	updated := routingentity.RouteHealthScore{AccountID: accountID, Route: route, Score: value, UpdatedUnix: now.Unix()}
	if err := updated.Validate(); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	index := -1
	for candidate, score := range scores {
		if score.AccountID == accountID && score.Route == route {
			index = candidate
			break
		}
	}
	if value == 0 {
		if index >= 0 {
			scores = append(scores[:index], scores[index+1:]...)
		}
	} else if index >= 0 {
		scores[index] = updated
	} else {
		scores = append(scores, updated)
	}
	scores = retainRouteHealth(scores, now)
	if err := store.writeRouteHealth(scores); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	return updated, nil
}

func (store *Store) AdjustRouteHealth(
	ctx context.Context,
	accountID, route string,
	delta int,
	now time.Time,
) (routingentity.RouteHealthScore, error) {
	if err := ctx.Err(); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	if err := store.prepare(); err != nil {
		return routingentity.RouteHealthScore{}, fmt.Errorf("prepare routing health store: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(store.root, "routing-health.guard"))
	if err != nil {
		return routingentity.RouteHealthScore{}, fmt.Errorf("lock routing health store: %w", err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	scores, err := store.readRouteHealth()
	if err != nil {
		return routingentity.RouteHealthScore{}, fmt.Errorf("read routing health store: %w", err)
	}
	updated := routingentity.RouteHealthScore{AccountID: accountID, Route: route, UpdatedUnix: now.Unix()}
	index := -1
	for candidate, score := range scores {
		if score.AccountID == accountID && score.Route == route {
			updated, index = score, candidate
			break
		}
	}
	if index < 0 && delta < 0 {
		return updated.Adjust(delta, now)
	}
	updated, err = updated.Adjust(delta, now)
	if err != nil {
		return routingentity.RouteHealthScore{}, err
	}
	if index >= 0 {
		scores[index] = updated
	} else {
		scores = append(scores, updated)
	}
	scores = retainRouteHealth(scores, now)
	if err := store.writeRouteHealth(scores); err != nil {
		return routingentity.RouteHealthScore{}, fmt.Errorf("write routing health store: %w", err)
	}
	return updated, nil
}

func (store *Store) readRouteHealth() ([]routingentity.RouteHealthScore, error) {
	path := filepath.Join(store.root, "route-health.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat routing health snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxRouteHealthBytes {
		return nil, errors.New("routing health snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open routing health snapshot: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxRouteHealthBytes+1))
	decoder.DisallowUnknownFields()
	var snapshot routeHealthSnapshot
	if decoder.Decode(&snapshot) != nil {
		return nil, errors.New("decode routing health snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("routing health snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Scores) > maxRouteHealthScores {
		return nil, errors.New("unsupported or oversized routing health snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Scores))
	for _, score := range snapshot.Scores {
		if err := score.Validate(); err != nil {
			return nil, err
		}
		key := score.AccountID + ":" + score.Route
		if seen[key] {
			return nil, errors.New("routing health snapshot has duplicate scores")
		}
		seen[key] = true
	}
	return snapshot.Scores, nil
}

func (store *Store) writeRouteHealth(scores []routingentity.RouteHealthScore) error {
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].AccountID != scores[j].AccountID {
			return scores[i].AccountID < scores[j].AccountID
		}
		return scores[i].Route < scores[j].Route
	})
	content, err := json.Marshal(routeHealthSnapshot{Version: 1, Scores: scores})
	if err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(filepath.Join(store.root, "route-health.json"), content)
	return err
}

func retainRouteHealth(scores []routingentity.RouteHealthScore, now time.Time) []routingentity.RouteHealthScore {
	cutoff := now.Add(-routingentity.RouteHealthRetention).Unix()
	active := scores[:0]
	for _, score := range scores {
		if score.UpdatedUnix > cutoff {
			active = append(active, score)
		}
	}
	if len(active) > maxRouteHealthScores {
		sort.Slice(active, func(i, j int) bool {
			if active[i].UpdatedUnix != active[j].UpdatedUnix {
				return active[i].UpdatedUnix > active[j].UpdatedUnix
			}
			if active[i].AccountID != active[j].AccountID {
				return active[i].AccountID < active[j].AccountID
			}
			return active[i].Route < active[j].Route
		})
		active = active[:maxRouteHealthScores]
	}
	return active
}
