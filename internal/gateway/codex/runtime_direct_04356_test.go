package codex

import (
	"slices"
	"testing"
)

func TestProdex04356DirectRuntimeNoProxyRemovesOnlyUpstreamProxyEnvironment(t *testing.T) {
	input := []string{
		"HTTP_PROXY=http://one",
		"HTTPS_PROXY=http://two",
		"ALL_PROXY=http://three",
		"http_proxy=http://four",
		"https_proxy=http://five",
		"all_proxy=http://six",
		"PROXY=http://seven",
		"proxy=http://eight",
		"NO_PROXY=localhost",
		"no_proxy=127.0.0.1",
		"OTHER=value",
	}
	got := removeUpstreamProxyEnvironment(input)
	for _, forbidden := range input[:8] {
		if slices.Contains(got, forbidden) {
			t.Fatalf("upstream proxy entry survived: %q in %#v", forbidden, got)
		}
	}
	for _, want := range input[8:] {
		if !slices.Contains(got, want) {
			t.Fatalf("non-upstream proxy entry removed: %q from %#v", want, got)
		}
	}
}
