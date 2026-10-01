package routing

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"sort"
	"time"
)

func sortRuntimeAccounts(accounts []proxymodel.Account) []proxymodel.Account {
	accounts = append([]proxymodel.Account(nil), accounts...)
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].RouteOrder > 0 && accounts[j].RouteOrder > 0 &&
			accounts[i].RouteOrder != accounts[j].RouteOrder {
			return accounts[i].RouteOrder < accounts[j].RouteOrder
		}
		if accounts[i].ID != accounts[j].ID {
			return accounts[i].ID < accounts[j].ID
		}
		if accounts[i].Enabled != accounts[j].Enabled {
			return accounts[i].Enabled
		}
		return accounts[i].Home < accounts[j].Home
	})
	unique := accounts[:0]
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if _, exists := seen[account.ID]; exists {
			continue
		}
		seen[account.ID] = struct{}{}
		unique = append(unique, account)
	}
	return unique
}

func (proxy *Router) candidates(accounts []proxymodel.Account, now time.Time) []proxymodel.Account {
	available := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.ID != "" && account.Home != "" && account.Enabled && !account.EligibleAfter.After(now) && !proxy.isQuarantined(account.ID, now) {
			available = append(available, account)
		}
	}
	if len(available) == 0 {
		return nil
	}
	proxy.mu.Lock()
	start := proxy.cursor % len(available)
	if !proxy.preferredUsed {
		proxy.preferredUsed = true
		for index, account := range available {
			if account.ID == proxy.preferred {
				start = index
				break
			}
		}
	}
	proxy.cursor = (start + 1) % len(available)
	proxy.mu.Unlock()
	ordered := make([]proxymodel.Account, 0, len(available))
	ordered = append(ordered, available[start:]...)
	ordered = append(ordered, available[:start]...)
	return ordered
}
