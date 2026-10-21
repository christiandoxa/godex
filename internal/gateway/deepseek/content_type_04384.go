package deepseek

import "strings"

// Prodex 0.438.4 lowercases response Content-Type using Rust's
// to_ascii_lowercase(), not Unicode casefolding. A U+017F LONG S in
// text/event-ſtream is not the ASCII SSE media subtype.
func deepSeekContentTypeIsSSE04384(value string) bool {
	return strings.Contains(deepSeekASCIILower04384(value), "text/event-stream")
}
