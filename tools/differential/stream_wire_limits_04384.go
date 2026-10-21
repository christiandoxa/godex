package main

func wireLimitMessage04384(scenario string) string {
	switch scenario {
	case "sse-line-limit":
		return "provider SSE line exceeded safe size limit (4194304)"
	case "sse-event-limit":
		return "provider SSE event exceeded safe size limit (4194304)"
	case "sse-comment-utf8":
		return "stream did not contain valid UTF-8"
	}
	return ""
}

func validWireLimitSSE04384(scenario, body string) bool {
	message := wireLimitMessage04384(scenario)
	_, err := canonicalFixtureStreamFailure04384(body, "provider_stream_error", message)
	return message != "" && err == nil
}

func equivalentWireLimitSSE04384(left, right string) bool {
	for _, scenario := range []string{"sse-line-limit", "sse-event-limit", "sse-comment-utf8"} {
		if validWireLimitSSE04384(scenario, left) && validWireLimitSSE04384(scenario, right) {
			return true
		}
	}
	return false
}
