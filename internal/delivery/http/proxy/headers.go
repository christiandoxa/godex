package proxy

import (
	"github.com/christiandoxa/godex/internal/helper/httpheader"
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
	connectionHeaders := httpheader.ConnectionTokens(source)
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
	connectionHeaders := httpheader.ConnectionTokens(responseHeaders)
	for key, values := range source {
		if shouldSkipResponseHeader(key, connectionHeaders) {
			continue
		}
		destination[key] = append([]string(nil), values...)
	}
}

func declareResponseTrailers(destination, responseHeaders, trailers http.Header) {
	connectionHeaders := httpheader.ConnectionTokens(responseHeaders)
	declared := make(map[string]bool)
	add := func(value string) {
		for _, name := range strings.Split(value, ",") {
			canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
			if canonical == "" || shouldSkipResponseHeader(canonical, connectionHeaders) || declared[canonical] {
				continue
			}
			destination.Add("Trailer", canonical)
			declared[canonical] = true
		}
	}
	for _, value := range headerValues(responseHeaders, "Trailer") {
		add(value)
	}
	for key := range trailers {
		add(key)
	}
}

func headerValues(headers http.Header, name string) []string {
	var values []string
	for key, current := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			values = append(values, current...)
		}
	}
	return values
}

func shouldSkipResponseHeader(key string, connectionHeaders map[string]bool) bool {
	name := strings.ToLower(strings.TrimSpace(key))
	return name == "content-length" || httpheader.IsHop(name) || connectionHeaders[http.CanonicalHeaderKey(key)]
}
