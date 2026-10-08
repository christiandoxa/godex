package session

// sessionIDMatchesSelector matches the Prodex 0.435.9 session-selector Mojo
// contract: compare the UTF-8 bytes, folding ASCII A-Z only. Do not use
// strings.EqualFold/ToLower, which also fold non-ASCII Unicode codepoints.
func sessionIDMatchesSelector(id, selector string, exact bool) bool {
	if len(selector) > len(id) || (exact && len(id) != len(selector)) {
		return false
	}
	for i := 0; i < len(selector); i++ {
		left, right := id[i], selector[i]
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		if left != right {
			return false
		}
	}
	return true
}
