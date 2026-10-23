package quota

import (
	"context"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type displayUsageGateway interface {
	FetchForDisplayAt(context.Context, string, string) (quotamodel.Usage, error)
}

func (status *Status) fetchDisplayUsage(ctx context.Context, home, upstream string) (quotamodel.Usage, error) {
	if gateway, ok := status.usage.(displayUsageGateway); ok {
		return status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
			return gateway.FetchForDisplayAt(ctx, home, upstream)
		})
	}
	return status.fetchHomeUsage(ctx, home, upstream)
}
