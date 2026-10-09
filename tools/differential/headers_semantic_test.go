package main

import (
	"net/http"
	"testing"
	"time"
)

func TestVerifiedHttpResponseTransportFingerprint(t *testing.T) {
	date := time.Now().UTC().Format(http.TimeFormat)
	prodex := http.Header{
		"Content-Type":   []string{"application/json; charset=utf-8"},
		"Content-Length": []string{"308"},
		"Date":           []string{date},
		"Server":         []string{"tiny-http (Rust)"},
	}
	godex := http.Header{
		"Content-Type":   []string{"application/json; charset=utf-8"},
		"Content-Length": []string{"308"},
		"Date":           []string{date},
	}
	if !equivalentClientHeaders(prodex, godex) {
		t.Fatal("verified default transport fingerprint rejected")
	}
	mutations := []struct {
		name   string
		change func(http.Header)
	}{
		{"content_type", func(h http.Header) { h.Set("Content-Type", "application/octet-stream") }},
		{"length", func(h http.Header) { h.Set("Content-Length", "999") }},
		{"auth", func(h http.Header) { h.Set("WWW-Authenticate", "Bearer realm=other") }},
		{"reason", func(h http.Header) { h.Set("Retry-After", "10") }},
		{"codex_metadata", func(h http.Header) { h.Set("X-Codex-Turn-State", "wrong") }},
		{"server_forgery", func(h http.Header) { h.Set("Server", "unknown-proxy") }},
		{"missing_date", func(h http.Header) { h.Del("Date") }},
		{"stale_date", func(h http.Header) { h.Set("Date", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)) }},
	}
	for _, fixture := range mutations {
		t.Run(fixture.name, func(t *testing.T) {
			changed := godex.Clone()
			fixture.change(changed)
			if equivalentClientHeaders(prodex, changed) {
				t.Fatalf("semantic header mutation %q was ignored", fixture.name)
			}
		})
	}
}

func TestVerifiedProviderUserAgentDuplicationOnly(t *testing.T) {
	ref := upstreamRequest{
		Method: http.MethodPost, Path: "/v1/chat/completions", Body: "{}",
		AuthOK: true,
		Headers: http.Header{
			"Accept":        []string{"text/event-stream, application/json", "application/json"},
			"Content-Type":  []string{"application/json", "application/json"},
			"Authorization": []string{"<redacted>"},
			"User-Agent":    []string{"Go-http-client/1.1", "Go-http-client/1.1"},
		},
	}
	candidate := ref
	candidate.Headers = ref.Headers.Clone()
	candidate.Headers["User-Agent"] = []string{"Go-http-client/1.1"}
	if !equivalentUpstreamRequests([]upstreamRequest{ref}, []upstreamRequest{candidate}) {
		t.Fatal("exact verified user-agent multiplicity rejected")
	}
	mutations := []struct {
		name   string
		mutate func(*upstreamRequest)
	}{
		{"other_user_agent", func(r *upstreamRequest) { r.Headers.Set("User-Agent", "evil-agent") }},
		{"model_body", func(r *upstreamRequest) { r.Body = `{"model":"wrong"}` }},
		{"wrong_endpoint", func(r *upstreamRequest) { r.Path = "/v1/wrong" }},
		{"invalid_auth", func(r *upstreamRequest) { r.AuthOK = false }},
		{"accept", func(r *upstreamRequest) { r.Headers.Set("Accept", "text/plain") }},
		{"secret", func(r *upstreamRequest) { r.Headers.Set("Authorization", "bad") }},
	}
	for _, fixture := range mutations {
		t.Run(fixture.name, func(t *testing.T) {
			bad := candidate
			bad.Headers = candidate.Headers.Clone()
			fixture.mutate(&bad)
			if equivalentUpstreamRequests([]upstreamRequest{ref}, []upstreamRequest{bad}) {
				t.Fatalf("semantic request mutation %q was hidden", fixture.name)
			}
		})
	}
}
