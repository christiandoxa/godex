package proxy

import (
	"net/http"
	"testing"
)

func TestResponseHeaderFilteringHandlesNonCanonicalConnectionKeys(t *testing.T) {
	destination := make(http.Header)
	source := http.Header{
		"connection":  {"x-local-hop"},
		"x-local-hop": {"strip-me"},
		"x-codex":     {"keep-me"},
	}
	copyResponseHeaders(destination, source)
	if destination.Get("x-local-hop") != "" || destination.Get("x-codex") != "keep-me" {
		t.Fatalf("filtered response headers = %#v", destination)
	}
}

func TestResponseTrailerDeclarationHandlesNonCanonicalTrailerKey(t *testing.T) {
	destination := make(http.Header)
	declareResponseTrailers(destination, http.Header{"trailer": {"x-upstream"}}, nil)
	if destination.Get("Trailer") != "X-Upstream" {
		t.Fatalf("declared trailers = %#v", destination)
	}
}
