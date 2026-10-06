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
