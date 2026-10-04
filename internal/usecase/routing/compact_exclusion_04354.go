package routing

import (
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func compactQuotaFallbackExhaustedForRequest(
	request proxymodel.Request,
	candidates []proxymodel.Account,
	failedAccountID string,
	excluded map[string]bool,
) bool {
	path := strings.TrimRight(request.Path, "/")
	if !strings.HasSuffix(path, "/responses/compact") {
		return false
	}
	for _, candidate := range candidates {
		if candidate.ID == failedAccountID || excluded[candidate.ID] {
			continue
		}
		return false
	}
	return true
}
