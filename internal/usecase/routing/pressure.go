package routing

import (
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	defaultInflightSoftLimit = 4
	maxRouteHealthScores     = 1024
)

type candidateLoad struct {
	backoff          candidateBackoffSortKey
	providerPriority uint8
	inflight         int
	health           uint32
	pressure         quotamodel.Pressure
	quotaSource      quotamodel.Source
	soft             bool
	promptPriority   uint8
	promptScore      uint64
	orderIndex       int
	jitter           uint64
	sourceIndex      int
}

type candidateRankContext struct {
	promptCacheKey    string
	selectionSequence uint64
}

func (router *Router) orderCandidates(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time) []proxymodel.Account {
	return router.orderCandidatesModeWithRank(accounts, selection, now, true, candidateRankContext{})
}

func (router *Router) orderCandidatesWithoutRotation(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time) []proxymodel.Account {
	return router.orderCandidatesModeWithRank(accounts, selection, now, false, candidateRankContext{})
}

func (router *Router) orderCandidatesMode(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time, consumeRotation bool) []proxymodel.Account {
	return router.orderCandidatesModeWithRank(accounts, selection, now, consumeRotation, candidateRankContext{})
}

func candidateRankContextForRequest(request proxymodel.Request) candidateRankContext {
	sequence := request.SelectionSequence
	if sequence == 0 {
		sequence = request.RequestID
	}
	return candidateRankContext{promptCacheKey: requestPromptCacheKey(request), selectionSequence: sequence}
}

func (router *Router) orderCandidatesModeWithRank(
	accounts []proxymodel.Account,
	selection quotamodel.Selection,
	now time.Time,
	consumeRotation bool,
	rank candidateRankContext,
) []proxymodel.Account {
	if len(accounts) < 2 {
		return accounts
	}
	accounts = append([]proxymodel.Account(nil), accounts...)
	loads := make(map[string]candidateLoad, len(accounts))

	router.mu.Lock()
	preferred := router.preferred
	usePreferred := consumeRotation && !router.preferredUsed
	softLimit := defaultInflightSoftLimit
	quotaChecks := make(map[string]quotaCheck, len(accounts))
	inflight := make(map[string]int, len(accounts))
	retryUntil := make(map[string]time.Time, len(accounts))
	for _, account := range accounts {
		quotaChecks[account.ID] = router.quotaChecks[quotaCheckKey{accountID: account.ID, selection: selection}]
		inflight[account.ID] = router.inflight[account.ID]
		if state, exists := router.quarantine[account.ID]; exists {
			if !state.until.After(now) {
				delete(router.quarantine, account.ID)
			} else if !state.authFailure {
				retryUntil[account.ID] = state.until
			}
		}
	}
	router.mu.Unlock()

	promptOwner := router.promptCacheOwner(rank.promptCacheKey, now)
	route := routeHealthRoute(selection.RouteKind)
	for index, account := range accounts {
		state, cached := quotaChecks[account.ID]
		fresh := cached && now.Sub(state.checkedAt) >= 0 && now.Sub(state.checkedAt) < quotaCheckFreshness
		pressure := state.pressure
		if !fresh || (!pressure.Known && pressure.Band == 0 && pressure.Total == 0) {
			pressure = unknownPressure()
		}
		circuitUntil := router.routeCircuitUntil(account.ID, selection, now)
		transportUntil := router.transportBackoffUntil(account.ID, selection, now)
		promptPriority, promptScore := promptCacheAffinitySortKey(rank.promptCacheKey, promptOwner, account.ID)
		orderIndex := account.RouteOrder
		if orderIndex <= 0 {
			orderIndex = index + 1
		}
		loads[account.ID] = candidateLoad{
			backoff:          profileBackoffSortKey(circuitUntil, transportUntil, retryUntil[account.ID], now),
			providerPriority: runtimeProviderPriority(account), inflight: inflight[account.ID],
			health: router.routeCompositeHealthScore(account.ID, route, now), pressure: pressure, quotaSource: state.source,
			soft:           inflight[account.ID] >= softLimit,
			promptPriority: promptPriority, promptScore: promptScore, orderIndex: orderIndex,
			jitter: selectionJitter(rank.selectionSequence, account.ID, selection.RouteKind), sourceIndex: index,
		}
	}

	ready := make([]proxymodel.Account, 0, len(accounts))
	fallback := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		load := loads[account.ID]
		if load.backoff.class == 0 && !load.soft {
			ready = append(ready, account)
		} else {
			fallback = append(fallback, account)
		}
	}
	sort.SliceStable(ready, func(i, j int) bool { return candidateReadyLess(ready[i], ready[j], loads, selection.RouteKind) })
	sort.SliceStable(fallback, func(i, j int) bool {
		left, right := loads[fallback[i].ID], loads[fallback[j].ID]
		if order := compareBackoffSortKey(left.backoff, right.backoff); order != 0 {
			return order < 0
		}
		return candidateReadyLess(fallback[i], fallback[j], loads, selection.RouteKind)
	})
	ordered := append(ready, fallback...)
	if len(ordered) == 0 {
		return ordered
	}

	if usePreferred {
		for index, account := range ordered {
			if account.ID == preferred && preferredCurrentCandidateAllowed(
				loads[account.ID], selection.RouteKind, rank.promptCacheKey, promptOwner, preferred, len(ready) > 1,
			) {
				router.mu.Lock()
				router.preferredUsed = true
				router.mu.Unlock()
				return rotateAccounts(ordered, index)
			}
		}
	}
	if len(ready) == 0 || !consumeRotation {
		return ordered
	}
	best := loads[ordered[0].ID]
	last := 1
	for last < len(ready) && candidateRankEqual(best, loads[ordered[last].ID], selection.RouteKind) {
		last++
	}
	if last < 2 {
		return ordered
	}
	router.mu.Lock()
	rotationStart := router.cursor % last
	router.cursor = (rotationStart + 1) % last
	router.mu.Unlock()
	return append(rotateAccounts(ordered[:last], rotationStart), ordered[last:]...)
}

func preferredCurrentCandidateAllowed(
	load candidateLoad,
	route quotamodel.RouteKind,
	promptCacheKey, promptOwner, preferred string,
	hasAlternative bool,
) bool {
	if load.backoff.class != 0 || load.soft || load.health > 0 {
		return false
	}
	if hasAlternative && (route == quotamodel.RouteKindResponses || route == quotamodel.RouteKindWebSocket) &&
		load.quotaSource == quotamodel.SourcePersistedSnapshot {
		return false
	}
	if !hasAlternative || strings.TrimSpace(promptCacheKey) == "" {
		return true
	}
	if route != quotamodel.RouteKindResponses && route != quotamodel.RouteKindWebSocket {
		return true
	}
	return strings.TrimSpace(promptOwner) == preferred
}

func candidateReadyLess(leftAccount, rightAccount proxymodel.Account, loads map[string]candidateLoad, route quotamodel.RouteKind) bool {
	left, right := loads[leftAccount.ID], loads[rightAccount.ID]
	if left.providerPriority != right.providerPriority {
		return left.providerPriority < right.providerPriority
	}
	if order := compareQuotaPressure(left.pressure, right.pressure); order != 0 {
		return order < 0
	}
	if order := compareQuotaSource(left.quotaSource, right.quotaSource, route); order != 0 {
		return order < 0
	}
	if left.inflight != right.inflight {
		return left.inflight < right.inflight
	}
	if left.health != right.health {
		return left.health < right.health
	}
	if left.promptPriority != right.promptPriority {
		return left.promptPriority < right.promptPriority
	}
	if left.promptScore != right.promptScore {
		return left.promptScore < right.promptScore
	}
	if left.orderIndex != right.orderIndex {
		return left.orderIndex < right.orderIndex
	}
	if left.jitter != right.jitter {
		return left.jitter < right.jitter
	}
	return left.sourceIndex < right.sourceIndex
}

func compareQuotaSource(left, right quotamodel.Source, route quotamodel.RouteKind) int {
	if route != quotamodel.RouteKindResponses && route != quotamodel.RouteKindWebSocket {
		return 0
	}
	rank := func(source quotamodel.Source) int {
		switch source {
		case quotamodel.SourceLive:
			return 0
		case quotamodel.SourcePersistedSnapshot:
			return 1
		default:
			return 2
		}
	}
	leftRank, rightRank := rank(left), rank(right)
	if leftRank < rightRank {
		return -1
	}
	if leftRank > rightRank {
		return 1
	}
	return 0
}

func unknownPressure() quotamodel.Pressure {
	return quotamodel.Pressure{
		Band: 4, Total: math.MaxInt64, Weekly: math.MaxInt64, FiveHour: math.MaxInt64,
		WeeklyResetAt: math.MaxInt64, FiveHourResetAt: math.MaxInt64,
	}
}

func compareQuotaPressure(left, right quotamodel.Pressure) int {
	leftValues := [...]int64{
		int64(left.Band), left.Total, left.Weekly, left.FiveHour,
		-left.ReserveFloor, -left.WeeklyRemaining, -left.FiveHourRemaining,
		left.WeeklyResetAt, left.FiveHourResetAt,
	}
	rightValues := [...]int64{
		int64(right.Band), right.Total, right.Weekly, right.FiveHour,
		-right.ReserveFloor, -right.WeeklyRemaining, -right.FiveHourRemaining,
		right.WeeklyResetAt, right.FiveHourResetAt,
	}
	for index := range leftValues {
		if leftValues[index] < rightValues[index] {
			return -1
		}
		if leftValues[index] > rightValues[index] {
			return 1
		}
	}
	return 0
}

func candidateRankEqual(left, right candidateLoad, route quotamodel.RouteKind) bool {
	return left.backoff.class == 0 && right.backoff.class == 0 && !left.soft && !right.soft &&
		left.providerPriority == right.providerPriority && left.inflight == right.inflight && left.health == right.health &&
		left.promptPriority == right.promptPriority && left.promptScore == right.promptScore &&
		compareQuotaSource(left.quotaSource, right.quotaSource, route) == 0 &&
		compareQuotaPressure(left.pressure, right.pressure) == 0
}

func runtimeProviderPriority(account proxymodel.Account) uint8 {
	kind := strings.ToLower(strings.TrimSpace(account.Provider.Kind))
	if kind == "" || kind == "openai" {
		return 0
	}
	return 1
}

func rotateAccounts(accounts []proxymodel.Account, start int) []proxymodel.Account {
	ordered := make([]proxymodel.Account, 0, len(accounts))
	ordered = append(ordered, accounts[start:]...)
	return append(ordered, accounts[:start]...)
}

func (router *Router) beginRequestInFlight(accountID string, selection quotamodel.Selection) func() {
	weight := routeInflightWeight(selection.RouteKind)
	router.mu.Lock()
	if router.inflight == nil {
		router.inflight = make(map[string]int)
	}
	router.inflight[accountID] += weight
	router.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			if router.inflight[accountID] <= weight {
				delete(router.inflight, accountID)
			} else {
				router.inflight[accountID] -= weight
			}
			router.mu.Unlock()
		})
	}
}

func routeInflightWeight(route quotamodel.RouteKind) int {
	if route == quotamodel.RouteKindResponses || route == quotamodel.RouteKindWebSocket {
		return 2
	}
	return 1
}

type inFlightBody struct {
	io.ReadCloser
	release func()
}

type inFlightDuplexBody struct {
	*inFlightBody
	duplex io.ReadWriteCloser
}

func (body *inFlightDuplexBody) Write(buffer []byte) (int, error) {
	return body.duplex.Write(buffer)
}

func (body *inFlightBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if err == io.EOF {
		body.release()
	}
	return count, err
}

func (body *inFlightBody) Close() error {
	err := body.ReadCloser.Close()
	body.release()
	return err
}
