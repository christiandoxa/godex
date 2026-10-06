package routing

import (
	"context"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type previousResponseFailureRepository interface {
	LoadPreviousResponseFailures(context.Context, time.Time) ([]routingentity.PreviousResponseFailure, error)
	RecordPreviousResponseFailure(context.Context, string, string, string, time.Time) (routingentity.PreviousResponseFailure, error)
	ClearPreviousResponseFailures(context.Context, string, string) error
}

type previousResponseFailureKey struct {
	accountID   string
	responseKey string
	route       string
}

func (router *Router) loadPreviousResponseFailures(ctx context.Context, now time.Time) error {
	repository, ok := router.state.(previousResponseFailureRepository)
	if !ok {
		return nil
	}
	failures, err := repository.LoadPreviousResponseFailures(ctx, now)
	if err != nil {
		return err
	}
	for _, failure := range failures {
		router.previousResponseFailures[previousResponseFailureKey{
			accountID: failure.AccountID, responseKey: failure.ResponseKey, route: failure.Route,
		}] = failure
	}
	return nil
}

func (router *Router) previousResponseFailureScore(
	accountID, responseID string,
	selection quotamodel.Selection,
	now time.Time,
) uint8 {
	route := routeHealthRoute(selection.RouteKind)
	if accountID == "" || responseID == "" || route == "" {
		return 0
	}
	key := previousResponseFailureKey{
		accountID: accountID, responseKey: affinityDigest("previous", responseID), route: route,
	}
	router.previousResponseFailureMu.Lock()
	failure, ok := router.previousResponseFailures[key]
	router.previousResponseFailureMu.Unlock()
	if !ok {
		return 0
	}
	return failure.Effective(now)
}

func (router *Router) hasPreviousResponseFailure(responseID string, selection quotamodel.Selection) bool {
	route := routeHealthRoute(selection.RouteKind)
	if responseID == "" || route == "" {
		return false
	}
	responseKey := affinityDigest("previous", responseID)
	now := router.now()
	router.previousResponseFailureMu.Lock()
	defer router.previousResponseFailureMu.Unlock()
	for key, failure := range router.previousResponseFailures {
		if key.responseKey == responseKey && key.route == route &&
			failure.Effective(now) >= routingentity.PreviousResponseFailureThreshold {
			return true
		}
	}
	return false
}

func (router *Router) recordPreviousResponseFailure(
	ctx context.Context,
	accountID, responseID string,
	selection quotamodel.Selection,
	now time.Time,
) uint8 {
	route := routeHealthRoute(selection.RouteKind)
	if accountID == "" || responseID == "" || route == "" {
		return 0
	}
	responseKey := affinityDigest("previous", responseID)
	key := previousResponseFailureKey{accountID: accountID, responseKey: responseKey, route: route}
	if repository, ok := router.state.(previousResponseFailureRepository); ok {
		if failure, err := repository.RecordPreviousResponseFailure(ctx, accountID, responseKey, route, now); err == nil {
			router.storePreviousResponseFailure(key, failure, now)
			return failure.Score
		}
	}
	router.previousResponseFailureMu.Lock()
	defer router.previousResponseFailureMu.Unlock()
	failure, exists := router.previousResponseFailures[key]
	if !exists {
		failure = routingentity.PreviousResponseFailure{
			AccountID: accountID, ResponseKey: responseKey, Route: route, UpdatedUnix: now.Unix(),
		}
	}
	updated, err := failure.Record(now)
	if err != nil {
		return 0
	}
	router.storePreviousResponseFailureLocked(key, updated, now)
	return updated.Score
}

func (router *Router) notePreviousResponseNotFound(
	ctx context.Context,
	account proxymodel.Account,
	responseID string,
	selection quotamodel.Selection,
	keys *affinityKeys,
) error {
	now := router.now()
	failures := router.recordPreviousResponseFailure(ctx, account.ID, responseID, selection, now)
	if failures < routingentity.PreviousResponseFailureThreshold {
		return nil
	}
	router.bumpRouteBadPairing(ctx, account.ID, selection, 1)
	releaseKeys := affinityKeys{previous: responseID}
	if keys != nil {
		releaseKeys.turn = keys.turn
		releaseKeys.session = keys.session
	}
	if err := router.affinity.releasePreviousResponse(ctx, responseID, account.ID, account.Home, releaseKeys); err != nil {
		return err
	}
	if keys != nil {
		keys.previous, keys.turn, keys.session = "", "", ""
	}
	return nil
}

func (router *Router) storePreviousResponseFailure(
	key previousResponseFailureKey,
	failure routingentity.PreviousResponseFailure,
	now time.Time,
) {
	router.previousResponseFailureMu.Lock()
	defer router.previousResponseFailureMu.Unlock()
	router.storePreviousResponseFailureLocked(key, failure, now)
}

func (router *Router) storePreviousResponseFailureLocked(
	key previousResponseFailureKey,
	failure routingentity.PreviousResponseFailure,
	now time.Time,
) {
	cutoff := now.Add(-routingentity.PreviousResponseFailureRetention).Unix()
	for cachedKey, cached := range router.previousResponseFailures {
		if cached.UpdatedUnix <= cutoff {
			delete(router.previousResponseFailures, cachedKey)
		}
	}
	if cached, exists := router.previousResponseFailures[key]; exists &&
		(cached.UpdatedUnix > failure.UpdatedUnix ||
			(cached.UpdatedUnix == failure.UpdatedUnix && cached.Score > failure.Score)) {
		return
	}
	router.previousResponseFailures[key] = failure
	if len(router.previousResponseFailures) <= routingentity.MaxPreviousResponseFailures {
		return
	}

	oldestKey, oldest := key, failure
	for candidateKey, candidate := range router.previousResponseFailures {
		older := candidate.UpdatedUnix < oldest.UpdatedUnix
		if candidate.UpdatedUnix == oldest.UpdatedUnix {
			switch {
			case candidateKey.accountID != oldestKey.accountID:
				older = candidateKey.accountID > oldestKey.accountID
			case candidateKey.route != oldestKey.route:
				older = candidateKey.route > oldestKey.route
			default:
				older = candidateKey.responseKey > oldestKey.responseKey
			}
		}
		if older {
			oldestKey, oldest = candidateKey, candidate
		}
	}
	delete(router.previousResponseFailures, oldestKey)
}

func (router *Router) clearPreviousResponseFailures(
	ctx context.Context,
	accountID, responseID string,
) {
	if accountID == "" || responseID == "" {
		return
	}
	responseKey := affinityDigest("previous", responseID)
	if repository, ok := router.state.(previousResponseFailureRepository); ok {
		_ = repository.ClearPreviousResponseFailures(ctx, accountID, responseKey)
	}
	router.previousResponseFailureMu.Lock()
	defer router.previousResponseFailureMu.Unlock()
	for key := range router.previousResponseFailures {
		if key.accountID == accountID && key.responseKey == responseKey {
			delete(router.previousResponseFailures, key)
		}
	}
}

func (router *Router) previousResponseFailureAccounts(
	accounts []proxymodel.Account,
	request proxymodel.Request,
	now time.Time,
) []proxymodel.Account {
	responseID := requestPreviousResponseID(request.Body)
	route := routeHealthRoute(request.QuotaSelection.RouteKind)
	if responseID == "" || route == "" {
		return accounts
	}
	eligible := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if router.previousResponseFailureScore(account.ID, responseID, request.QuotaSelection, now) <
			routingentity.PreviousResponseFailureThreshold {
			eligible = append(eligible, account)
		}
	}
	return eligible
}

func (router *Router) requestCandidatesForRequest(
	accounts []proxymodel.Account,
	request proxymodel.Request,
	now time.Time,
) []proxymodel.Account {
	return router.requestCandidatesForRequestMode(accounts, request, now, true)
}

func (router *Router) requestCandidatesForRequestWithoutRotation(
	accounts []proxymodel.Account,
	request proxymodel.Request,
	now time.Time,
) []proxymodel.Account {
	return router.requestCandidatesForRequestMode(accounts, request, now, false)
}

func (router *Router) requestCandidatesForRequestMode(
	accounts []proxymodel.Account,
	request proxymodel.Request,
	now time.Time,
	consumeRotation bool,
) []proxymodel.Account {
	candidates := router.requestCandidatesModeWithRank(
		accounts, request.QuotaSelection, now, consumeRotation, candidateRankContextForRequest(request),
	)
	return router.previousResponseFailureAccounts(candidates, request, now)
}
