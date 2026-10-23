package quota

import (
	"context"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type workspaceGateway interface {
	FetchWorkspaceAt(context.Context, string, string) (*quotamodel.Workspace, error)
}

func (status *Status) fetchWorkspace(ctx context.Context, home, upstream string) *quotamodel.Workspace {
	if gateway, ok := status.usage.(workspaceGateway); ok {
		workspace, err := gateway.FetchWorkspaceAt(ctx, home, upstream)
		if err == nil {
			return workspace
		}
	}
	return nil
}
