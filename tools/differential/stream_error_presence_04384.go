package main

func deepSeekStreamErrorFixture04384(scenario string) (raw, code, message string, ok bool) {
	code, message = "provider_stream_error", "Provider stream returned an embedded error"
	switch scenario {
	case "deepseek-sse-error-presence":
		raw = `{"type":null,"code":"SECONDARY","message":null,"detail":"must-not-borrow"}`
	case "deepseek-sse-error-i64":
		raw, code = `{"code":9223372036854775807}`, "9223372036854775807"
	case "deepseek-sse-error-float":
		raw = `{"code":1.0}`
	default:
		return "", "", "", false
	}
	return raw, code, message, true
}

func validFixtureStreamErrorPresence04384(scenario, wire string) bool {
	_, code, message, ok := deepSeekStreamErrorFixture04384(scenario)
	if !ok {
		return false
	}
	_, err := canonicalFixtureStreamFailure04384(wire, code, message)
	return err == nil
}

func equivalentFixtureStreamErrorPresence04384(left, right string) bool {
	for _, scenario := range []string{"deepseek-sse-error-presence", "deepseek-sse-error-i64", "deepseek-sse-error-float"} {
		if validFixtureStreamErrorPresence04384(scenario, left) && validFixtureStreamErrorPresence04384(scenario, right) {
			return true
		}
	}
	return false
}
