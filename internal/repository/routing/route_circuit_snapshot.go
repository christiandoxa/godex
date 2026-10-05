package routing

import (
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
)

const (
	maxRouteCircuits     = 4096
	maxRouteCircuitBytes = 1 << 20
)

type routeCircuitSnapshot struct {
	Version  int                          `json:"version"`
	Circuits []routingentity.RouteCircuit `json:"circuits"`
}

func (store *Store) readRouteCircuits() (routeCircuitSnapshot, error) {
	snapshot := routeCircuitSnapshot{Version: 1}
	path := filepath.Join(store.root, "route-circuits.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, fmt.Errorf("stat routing circuit snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxRouteCircuitBytes {
		return snapshot, errors.New("routing circuit snapshot must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return snapshot, fmt.Errorf("open routing circuit snapshot: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxRouteCircuitBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, errors.New("decode routing circuit snapshot")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return snapshot, errors.New("routing circuit snapshot has trailing data")
	}
	if snapshot.Version != 1 || len(snapshot.Circuits) > maxRouteCircuits {
		return snapshot, errors.New("unsupported or oversized routing circuit snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Circuits))
	for _, circuit := range snapshot.Circuits {
		if err := circuit.Validate(); err != nil {
			return snapshot, err
		}
		key := circuit.AccountID + "\x00" + circuit.Route
		if seen[key] {
			return snapshot, errors.New("routing circuit snapshot has duplicate routes")
		}
		seen[key] = true
	}
	return snapshot, nil
}

func (store *Store) writeRouteCircuits(circuits []routingentity.RouteCircuit) error {
	sort.Slice(circuits, func(i, j int) bool {
		if circuits[i].AccountID != circuits[j].AccountID {
			return circuits[i].AccountID < circuits[j].AccountID
		}
		return circuits[i].Route < circuits[j].Route
	})
	content, err := json.Marshal(routeCircuitSnapshot{Version: 1, Circuits: circuits})
	if err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(filepath.Join(store.root, "route-circuits.json"), content)
	return err
}

func retainRouteCircuits(circuits []routingentity.RouteCircuit, now time.Time) []routingentity.RouteCircuit {
	cutoff := now.Add(-routingentity.RouteHealthRetention).Unix()
	active := circuits[:0]
	for _, circuit := range circuits {
		if circuit.StageUpdatedUnix > cutoff {
			active = append(active, circuit)
		}
	}
	if len(active) > maxRouteCircuits {
		sort.Slice(active, func(i, j int) bool {
			if active[i].StageUpdatedUnix != active[j].StageUpdatedUnix {
				return active[i].StageUpdatedUnix > active[j].StageUpdatedUnix
			}
			if active[i].AccountID != active[j].AccountID {
				return active[i].AccountID < active[j].AccountID
			}
			return active[i].Route < active[j].Route
		})
		active = active[:maxRouteCircuits]
	}
	return active
}
