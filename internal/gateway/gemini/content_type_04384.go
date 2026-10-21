package gemini

import "strings"

// The 0.438.4 local-rewrite response dispatcher performs Rust's
// to_ascii_lowercase().contains("text/event-stream"). Unicode casefolding
// must not let U+017F long-s turn `text/event-ſtream` into an SSE MIME.
func geminiContentTypeIsSSE04384(raw string) bool {
	return strings.Contains(geminiASCIILower04384(raw), "text/event-stream")
}
