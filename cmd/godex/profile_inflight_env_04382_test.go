package main

import (
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	"testing"
)

func TestProdex04382RuntimeRoutingPreservesConfiguredProfileInflightHardLimit(t *testing.T) {
	for _, hardLimit := range []int{0, 2, 8, 17} {
		result := runtimeRoutingConfig(proxyconfig.Config{ProfileInflightHardLimit: hardLimit}, runtimeRouterDependencies{}, nil)
		if result.ProfileInflightHardLimit != hardLimit {
			t.Fatalf("router hard limit=%d, want=%d", result.ProfileInflightHardLimit, hardLimit)
		}
	}
}
