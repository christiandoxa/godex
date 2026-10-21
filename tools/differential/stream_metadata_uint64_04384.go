package main

import "strings"

const exactStreamMetadata04384 = `{"annotations":[{"source_index":18446744073709551615}],"finish_reason":"stop","logprobs":{"token_count":9007199254740993}}`

// Check the exact integer tokens before applying the ordinary full SSE oracle.
func canonicalStreamMetadata04384(body string) (string, bool) {
	if strings.Count(body, exactStreamMetadata04384) != 1 {
		return "", false
	}
	normalized := strings.Replace(body, exactStreamMetadata04384, `{"finish_reason":"stop"}`, 1)
	canonical, err := canonicalFixtureSSE(normalized)
	return canonical, err == nil
}

func equivalentStreamMetadata04384(left, right string) bool {
	a, leftOK := canonicalStreamMetadata04384(left)
	b, rightOK := canonicalStreamMetadata04384(right)
	return leftOK && rightOK && a == b
}
