package codex

import (
	"context"
	"path/filepath"
)

func (process *CodexProcess) ReadAuthSnapshot(ctx context.Context, home string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return readPrivateAuthFile(filepath.Join(home, "auth.json"))
}
