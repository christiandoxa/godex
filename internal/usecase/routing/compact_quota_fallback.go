package routing

import proxymodel "github.com/christiandoxa/godex/internal/model/proxy"

func compactQuotaFallbackExhausted(
	candidates []proxymodel.Account,
	failedAccountID string,
	excluded map[string]bool,
) bool {
	for _, candidate := range candidates {
		if candidate.ID != failedAccountID && !excluded[candidate.ID] {
			return false
		}
	}
	return true
}
