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
	backoff          time.Duration
	providerPriority uint8
	inflight         int
	health           uint32
	pressure         quotamodel.Pressure
	soft             bool
}

func (router *Router) orderCandidates(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time) []proxymodel.Account {
	return router.orderCandidatesMode(accounts, selection, now, true)
}

func (router *Router) orderCandidatesWithoutRotation(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time) []proxymodel.Account {
	return router.orderCandidatesMode(accounts, selection, now, false)
}

func (router *Router) orderCandidatesMode(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time, consumeRotation bool) []proxymodel.Account {
	if len(accounts) < 2 {
		return accounts
	}
	loads := make(map[string]candidateLoad, len(accounts))
	router.mu.Lock()
	preferred := router.preferred
	usePreferred := consumeRotation && !router.preferredUsed
	softLimit := defaultInflightSoftLimit
	for _, account := range accounts {
		state, cached := router.quotaChecks[quotaCheckKey{accountID: account.ID, selection: selection}]
		fresh := cached && now.Sub(state.checkedAt) >= 0 && now.Sub(state.checkedAt) < quotaCheckFreshness
		pressure := state.pressure
		if !fresh || (!pressure.Known && pressure.Band == 0 && pressure.Total == 0) {
			pressure = unknownPressure()
		}
		backoff := time.Duration(0)
		if state, exists := router.quarantine[account.ID]; exists {
			if state.until.After(now) {
				backoff = state.until.Sub(now)
			} else {
				delete(router.quarantine, account.ID)
			}
		}
		transportKey := routeHealthKey{accountID: account.ID, route: routeHealthRoute(selection.RouteKind)}
		if state, exists := router.transportBackoffs[transportKey]; exists {
			if remaining := state.Remaining(now); remaining > backoff {
				backoff = remaining
			} else if remaining == 0 {
				delete(router.transportBackoffs, transportKey)
			}
		}
		if remaining := router.routeCircuits[transportKey].Remaining(now); remaining > backoff {
			backoff = remaining
		}
		loads[account.ID] = candidateLoad{
			backoff:          backoff,
			providerPriority: runtimeProviderPriority(account),
			inflight:         router.inflight[account.ID],
			health:           router.routeCompositeHealthScoreLocked(account.ID, routeHealthRoute(selection.RouteKind), now),
			pressure:         pressure,
			soft:             router.inflight[account.ID] >= softLimit,
		}
	}
	router.mu.Unlock()

	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := loads[accounts[i].ID], loads[accounts[j].ID]
		if left.backoff != right.backoff {
			if left.backoff == 0 {
				return true
			}
			if right.backoff == 0 {
				return false
			}
			return left.backoff < right.backoff
		}
		if left.soft != right.soft {
			return !left.soft
		}
		if left.providerPriority != right.providerPriority {
			return left.providerPriority < right.providerPriority
		}
		if order := compareQuotaPressure(left.pressure, right.pressure); order != 0 {
			return order < 0
		}
		if left.inflight != right.inflight {
			return left.inflight < right.inflight
		}
		if left.health != right.health {
			return left.health < right.health
		}
		if accounts[i].RouteOrder > 0 && accounts[j].RouteOrder > 0 && accounts[i].RouteOrder != accounts[j].RouteOrder {
			return accounts[i].RouteOrder < accounts[j].RouteOrder
		}
		return accounts[i].ID < accounts[j].ID
	})

	if usePreferred {
		for index, account := range accounts {
			if account.ID == preferred {
				if loads[account.ID].backoff == 0 {
					router.mu.Lock()
					router.preferredUsed = true
					router.mu.Unlock()
					return rotateAccounts(accounts, index)
				}
				break
			}
		}
	}
	best := loads[accounts[0].ID]
	if best.backoff > 0 || !consumeRotation {
		return accounts
	}
	last := 1
	for last < len(accounts) && candidateRankEqual(best, loads[accounts[last].ID]) {
		last++
	}
	if last < 2 {
		return accounts
	}
	router.mu.Lock()
	start := router.cursor % last
	router.cursor = (start + 1) % last
	router.mu.Unlock()
	return append(rotateAccounts(accounts[:last], start), accounts[last:]...)
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

func candidateRankEqual(left, right candidateLoad) bool {
	return left.backoff == right.backoff && left.soft == right.soft && left.providerPriority == right.providerPriority &&
		left.inflight == right.inflight && left.health == right.health && compareQuotaPressure(left.pressure, right.pressure) == 0
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
