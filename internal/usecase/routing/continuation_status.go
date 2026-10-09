package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

type continuationStatusRepository interface {
	LoadContinuationStatuses(context.Context, time.Time) ([]routingentity.ContinuationStatus, error)
	SaveContinuationStatus(context.Context, routingentity.ContinuationStatus, time.Time) error
}

func (store *affinityStore) loadContinuationStatuses(ctx context.Context, now time.Time) error {
	repository, ok := store.repository.(continuationStatusRepository)
	if !ok {
		return nil
	}
	statuses, err := repository.LoadContinuationStatuses(ctx, now)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, persisted := range statuses {
		status := continuationStatus{
			kind:           persisted.Kind,
			state:          continuationDead,
			lastTouchedAt:  persisted.UpdatedUnix,
			lastNotFoundAt: persisted.UpdatedUnix,
			notFoundStreak: continuationSuspectLimit,
		}
		current, exists := store.statuses[persisted.Key]
		if exists && current.lastTouchedAt >= status.lastTouchedAt {
			continue
		}
		store.statuses[persisted.Key] = status
	}
	store.pruneContinuationStatusesLocked(now)
	return nil
}

func (store *affinityStore) persistContinuationDeadLocked(
	ctx context.Context,
	kind, key string,
	now time.Time,
) error {
	if !store.writesEnabled() {
		return nil
	}
	repository, ok := store.repository.(continuationStatusRepository)
	if !ok {
		return nil
	}
	status, ok := store.statuses[key]
	if !ok || status.kind != kind || status.state != continuationDead {
		return nil
	}
	return repository.SaveContinuationStatus(ctx, routingentity.ContinuationStatus{
		Kind: kind, Key: key, State: "dead", UpdatedUnix: status.lastTouchedAt,
	}, now)
}

func (router *Router) loadContinuationStatuses() error {
	if router.affinity.repository == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return router.affinity.loadContinuationStatuses(ctx, router.now())
}

func (router *Router) initializeAffinityPersistence(config Config) error {
	router.affinity.repository, router.affinity.clock = config.Bindings, router.now
	router.affinity.persistenceEnabled = router.persistenceWritesEnabled
	return router.loadContinuationStatuses()
}
