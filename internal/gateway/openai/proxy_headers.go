package openai

import (
	"net/http"
	"strings"
)

func hasHeader(headers http.Header, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func copyResponseHeaders(destination, source http.Header) {
	connectionHeaders := connectionHeaderTokens(source)
	for key, values := range source {
		if shouldSkipResponseHeader(key, connectionHeaders) {
			continue
		}
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func copyTrailers(destination, responseHeaders, source http.Header) {
	connectionHeaders := connectionHeaderTokens(responseHeaders)
	for key, values := range source {
		if isHopHeader(key) || connectionHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		destination[key] = append([]string(nil), values...)
	}
}

func declareResponseTrailers(destination, responseHeaders, trailers http.Header) {
	connectionHeaders := connectionHeaderTokens(responseHeaders)
	declared := make(map[string]bool)
	add := func(value string) {
		for _, name := range strings.Split(value, ",") {
			canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
			if canonical == "" || isHopHeader(canonical) || connectionHeaders[canonical] || declared[canonical] {
				continue
			}
			destination.Add("Trailer", canonical)
			declared[canonical] = true
		}
	}
	for _, value := range responseHeaders.Values("Trailer") {
		add(value)
	}
	for key := range trailers {
		add(key)
	}
}

func removeHopHeaders(headers http.Header) {
	connectionHeaders := connectionHeaderTokens(headers)
	for key := range headers {
		name := strings.ToLower(strings.TrimSpace(key))
		if isRequestTransportHeader(name) || connectionHeaders[http.CanonicalHeaderKey(key)] ||
			strings.HasPrefix(name, "sec-websocket-") ||
			strings.HasPrefix(name, "x-prodex-internal-") ||
			name == "authorization" || name == "chatgpt-account-id" {
			delete(headers, key)
		}
	}
}

func connectionHeaderTokens(headers http.Header) map[string]bool {
	tokens := make(map[string]bool)
	for _, value := range headers.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			canonical := http.CanonicalHeaderKey(strings.TrimSpace(token))
			if canonical != "" {
				tokens[canonical] = true
			}
		}
	}
	return tokens
}

func isRequestTransportHeader(name string) bool {
	switch name {
	case "connection", "content-length", "host", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func shouldSkipResponseHeader(key string, connectionHeaders map[string]bool) bool {
	name := strings.ToLower(strings.TrimSpace(key))
	return isHopHeader(name) || connectionHeaders[http.CanonicalHeaderKey(key)]
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
