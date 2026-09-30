package codex

import (
	"context"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (process *CodexProcess) ReadAuth(ctx context.Context, home string) (proxymodel.Auth, error) {
	if err := ctx.Err(); err != nil {
		return proxymodel.Auth{}, err
	}
	auth, err := ReadUsageAuth(home)
	return proxymodel.Auth{AccessToken: auth.AccessToken, AccountID: auth.AccountID}, err
}
