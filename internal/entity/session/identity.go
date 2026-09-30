package session

import "encoding/hex"

// ValidID accepts only Codex UUID thread identifiers, safe as CLI positionals.
func ValidID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for _, i := range []int{8, 13, 18, 23} {
		if id[i] != '-' {
			return false
		}
	}
	_, err := hex.DecodeString(id[:8] + id[9:13] + id[14:18] + id[19:23] + id[24:])
	return err == nil
}
