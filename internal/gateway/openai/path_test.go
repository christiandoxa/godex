package openai

import (
	"testing"
)

func TestUpstreamPathMatchesCodexMount(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{name: "responses", base: "/backend-api", path: "/backend-api/godex/responses", want: "/backend-api/codex/responses"},
		{name: "legacy version", base: "/backend-api", path: "/backend-api/godex/v1/responses", want: "/backend-api/codex/responses"},
		{name: "already normalized", base: "/backend-api", path: "/backend-api/codex/responses", want: "/backend-api/codex/responses"},
		{name: "legacy prodex alias", base: "/backend-api", path: "/backend-api/prodex/responses", want: "/backend-api/codex/responses"},
		{name: "custom base", base: "/backend-api-v2", path: "/backend-api/godex/responses", want: "/backend-api-v2/backend-api/codex/responses"},
		{name: "standard v1", base: "/backend-api", path: "/v1/responses", want: "/backend-api/v1/responses"},
		{name: "pathless base", base: "", path: "/v1/responses", want: "/v1/responses"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := upstreamPath(testCase.base, testCase.path); got != testCase.want {
				t.Fatalf("upstreamPath(%q, %q) = %q, want %q", testCase.base, testCase.path, got, testCase.want)
			}
		})
	}
}

func TestProtectDotSegmentsKeepsUpstreamPathOpaque(t *testing.T) {
	for _, testCase := range []struct {
		name, path, wantPath, wantRaw string
	}{
		{name: "plain parent", path: "/backend-api/codex/../secret", wantPath: "/backend-api/codex/%2e%2e/secret", wantRaw: "/backend-api/codex/%252e%252e/secret"},
		{name: "encoded parent", path: "/backend-api/codex/%2e%2e/secret", wantPath: "/backend-api/codex/%2e%2e/secret", wantRaw: "/backend-api/codex/%252e%252e/secret"},
		{name: "ordinary", path: "/backend-api/codex/responses", wantPath: "/backend-api/codex/responses", wantRaw: "/backend-api/codex/responses"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path, raw := protectDotSegments(testCase.path, "")
			if path != testCase.wantPath || raw != testCase.wantRaw {
				t.Fatalf("protectDotSegments(%q) = %q/%q, want %q/%q", testCase.path, path, raw, testCase.wantPath, testCase.wantRaw)
			}
		})
	}
}
