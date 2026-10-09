package main

import (
	"net/http"
	"reflect"
	"time"
)

// This is a narrow, fixture-specific transport comparison. It does not
// normalize authorization, model routing, content framing, retry headers,
// Codex metadata, or arbitrary provider headers.
func equivalentClientHeaders(reference, candidate http.Header) bool {
	left, right := canonicalHeaders(reference), canonicalHeaders(candidate)
	if !equivalentFreshDate(left, right) {
		return false
	}
	left.Del("Date")
	right.Del("Date")
	// Prodex 0.436.1 uses tiny-http (Rust). Godex's Go HTTP server has no
	// default Server header. Only this exact unmodified fingerprint pair is
	// accepted; custom upstream Server values must still match exactly.
	if left.Get("Server") == "tiny-http (Rust)" && right.Get("Server") == "" {
		left.Del("Server")
	}
	return reflect.DeepEqual(left, right)
}

func equivalentFreshDate(reference, candidate http.Header) bool {
	left, hasLeft := reference["Date"]
	right, hasRight := candidate["Date"]
	if !hasLeft && !hasRight {
		return true
	}
	if !hasLeft || !hasRight || len(left) != 1 || len(right) != 1 {
		return false
	}
	now := time.Now()
	for _, value := range []string{left[0], right[0]} {
		date, err := http.ParseTime(value)
		if err != nil || now.Sub(date) > 2*time.Minute || date.Sub(now) > 2*time.Minute {
			return false
		}
	}
	return true
}

func equivalentUpstreamRequests(reference, candidate []upstreamRequest) bool {
	if len(reference) != len(candidate) {
		return false
	}
	for index, left := range reference {
		right := candidate[index]
		if left.Method != right.Method || left.Path != right.Path ||
			left.Body != right.Body || left.AuthOK != right.AuthOK {
			return false
		}
		referenceHeaders := canonicalHeaders(left.Headers)
		candidateHeaders := canonicalHeaders(right.Headers)
		// The tagged Rust HTTP client adds Go-http-client/1.1 twice while
		// Godex forwards one occurrence. The exact identical default is an
		// implementation fingerprint, not permission to ignore arbitrary
		// User-Agent changes or any other provider request header.
		if reflect.DeepEqual(referenceHeaders.Values("User-Agent"), []string{"Go-http-client/1.1", "Go-http-client/1.1"}) &&
			reflect.DeepEqual(candidateHeaders.Values("User-Agent"), []string{"Go-http-client/1.1"}) {
			referenceHeaders["User-Agent"] = []string{"Go-http-client/1.1"}
		}
		if !reflect.DeepEqual(referenceHeaders, candidateHeaders) {
			return false
		}
	}
	return true
}

func canonicalHeaders(source http.Header) http.Header {
	target := make(http.Header, len(source))
	for key, values := range source {
		canonical := http.CanonicalHeaderKey(key)
		target[canonical] = append(target[canonical], values...)
	}
	return target
}
