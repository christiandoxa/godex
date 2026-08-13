package account

import (
	"fmt"
	"sort"
	"strings"
	"time"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func resolveIndex(accounts []entity.Account, selector string) (int, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return -1, fmt.Errorf("account selector is required")
	}

	match := -1
	for index, account := range accounts {
		if !account.Matches(selector) {
			continue
		}
		if match >= 0 {
			return -1, fmt.Errorf("account selector %q is ambiguous", selector)
		}
		match = index
	}
	if match < 0 {
		return -1, fmt.Errorf("account %q was not found", selector)
	}
	return match, nil
}

func selectIndex(state stateFile, selector string) (int, error) {
	if strings.TrimSpace(selector) != "" {
		return resolveIndex(state.Accounts, selector)
	}

	enabled := orderedEnabledIndexes(state.Accounts)
	if len(enabled) == 0 {
		return -1, fmt.Errorf("no enabled accounts; run `godex login`")
	}
	return enabled[state.RotationCursor%uint64(len(enabled))], nil
}

func identityIndex(accounts []entity.Account, candidate entity.Account) int {
	identity := entity.Identity{
		Email:            candidate.Email,
		ChatGPTAccountID: candidate.ChatGPTAccountID,
	}
	match := -1
	for index, account := range accounts {
		if account.SameIdentity(identity) {
			if match >= 0 {
				return -2
			}
			match = index
		}
	}
	return match
}

func availableDefaultName(accounts []entity.Account, base string) string {
	if !nameInUse(accounts, base, -1) {
		return base
	}
	for suffixNumber := 2; ; suffixNumber++ {
		suffix := fmt.Sprintf("-%d", suffixNumber)
		trimmedBase := strings.TrimRight(base, "._-")
		if maximum := 48 - len(suffix); len(trimmedBase) > maximum {
			trimmedBase = strings.TrimRight(trimmedBase[:maximum], "._-")
		}
		candidate := trimmedBase + suffix
		if !nameInUse(accounts, candidate, -1) {
			return candidate
		}
	}
}

func nameInUse(accounts []entity.Account, name string, ignoredIndex int) bool {
	for index, account := range accounts {
		if index != ignoredIndex && strings.EqualFold(account.Name, name) {
			return true
		}
	}
	return false
}

func ensureUniqueName(accounts []entity.Account, candidate entity.Account, ignoredIndex int) error {
	for index, account := range accounts {
		if index != ignoredIndex && strings.EqualFold(account.Name, candidate.Name) {
			return fmt.Errorf("account name %q is already used", candidate.Name)
		}
	}
	return nil
}

func nextCursor(accounts []entity.Account, selectedIndex int) uint64 {
	ordered := orderedEnabledIndexes(accounts)
	if len(ordered) == 0 {
		return 0
	}
	for position, index := range ordered {
		if index == selectedIndex {
			return uint64((position + 1) % len(ordered))
		}
	}
	return 0
}

func enabledPosition(accounts []entity.Account, selectedIndex int) uint64 {
	for position, index := range orderedEnabledIndexes(accounts) {
		if index == selectedIndex {
			return uint64(position)
		}
	}
	return 0
}

func repairSelection(state *stateFile, removedID string) {
	repairSelectionAfterRemove(state, removedID, "")
}

func cursorAccountID(state stateFile) string {
	enabled := orderedEnabledIndexes(state.Accounts)
	if len(enabled) == 0 {
		return ""
	}
	return state.Accounts[enabled[state.RotationCursor%uint64(len(enabled))]].ID
}

func accountPosition(accounts []entity.Account, accountID string) uint64 {
	for index, account := range accounts {
		if account.ID == accountID {
			return enabledPosition(accounts, index)
		}
	}
	return 0
}

func repairSelectionAfterRemove(state *stateFile, removedID, nextID string) {
	if state.ActiveAccountID == removedID {
		ordered := orderedEnabledIndexes(state.Accounts)
		state.ActiveAccountID = ""
		if len(ordered) > 0 {
			state.ActiveAccountID = state.Accounts[ordered[0]].ID
			state.RotationCursor = 0
		}
		return
	}

	ordered := orderedEnabledIndexes(state.Accounts)
	enabledCount := len(ordered)
	if enabledCount == 0 {
		state.RotationCursor = 0
		return
	}
	if nextID != "" && nextID != removedID {
		for index, account := range state.Accounts {
			if account.Enabled && account.ID == nextID {
				state.RotationCursor = enabledPosition(state.Accounts, index)
				return
			}
		}
	}
	state.RotationCursor %= uint64(enabledCount)
}

func orderedEnabledIndexes(accounts []entity.Account) []int {
	ordered := make([]int, 0, len(accounts))
	for index, account := range accounts {
		if account.Enabled {
			ordered = append(ordered, index)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := accounts[ordered[i]], accounts[ordered[j]]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})
	return ordered
}

func maxTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}
