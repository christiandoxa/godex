package codex

import (
	"context"
	"errors"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

func (process *CodexProcess) InspectAuthJSON(ctx context.Context, content []byte) (accountentity.Identity, error) {
	if err := ctx.Err(); err != nil {
		return accountentity.Identity{}, err
	}
	if len(content) == 0 || len(content) > maxAuthFileSize {
		return accountentity.Identity{}, errors.New("Codex auth snapshot is invalid or too large")
	}
	return chatGPTIdentity(content)
}
