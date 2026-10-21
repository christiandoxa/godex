package main

import "strings"

// Tagged serde emits exactly this integer zero object for a present null usage.
func normalizeDeepSeekNullUsage04384(body string) (string, bool) {
	const zero = `"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`
	const ordinary = `"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}`
	if strings.Count(body, zero) != 1 {
		return "", false
	}
	return strings.Replace(body, zero, ordinary, 1), true
}

func validDeepSeekNullUsage04384(body string, stream bool) bool {
	normalized, ok := normalizeDeepSeekNullUsage04384(body)
	if !ok {
		return false
	}
	if stream {
		return validFixtureSSE(normalized)
	}
	return validFixtureResponse(normalized)
}

func equivalentDeepSeekNullUsageSSE04384(left, right string) bool {
	a, leftOK := normalizeDeepSeekNullUsage04384(left)
	b, rightOK := normalizeDeepSeekNullUsage04384(right)
	return leftOK && rightOK && equivalentFixtureSSE(a, b)
}
