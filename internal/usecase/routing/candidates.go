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
		if account.ID == "" || account.Home == "" || !account.Enabled || account.EligibleAfter.After(now) {
			continue
		}
		if proxy.authFailureQuarantined(account.ID, now) ||
			(proxy.quotaBlockedAccount(account.ID) && proxy.isQuarantined(account.ID, now)) {
			continue
		}
		available = append(available, account)
	}
	return available
}
