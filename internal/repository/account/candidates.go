package account

import (
	"context"
	"fmt"
	"strings"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

// LaunchCandidates returns the deterministic launch order without mutating
// active-account, last-used, or rotation state. Selection is committed later by
// SelectForLaunch after runtime preflight has chosen an eligible candidate.
func (store *FileStore) LaunchCandidates(ctx context.Context, selector string) ([]entity.Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(selector) != "" {
		index, err := resolveIndex(state.Accounts, selector)
		if err != nil {
			return nil, err
		}
		account := state.Accounts[index]
		if !account.Enabled {
			return nil, fmt.Errorf("account %q is disabled", account.Name)
		}
		return []entity.Account{account}, nil
	}

	ordered := orderedEnabledIndexes(state.Accounts)
	if len(ordered) == 0 {
		return nil, fmt.Errorf("no enabled accounts; run `godex login`")
	}
	start := int(state.RotationCursor % uint64(len(ordered)))
	candidates := make([]entity.Account, 0, len(ordered))
	for offset := range ordered {
		index := ordered[(start+offset)%len(ordered)]
		candidates = append(candidates, state.Accounts[index])
	}
	return candidates, nil
}
