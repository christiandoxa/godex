package main

import (
	"slices"
	"testing"
)

func TestProdex04380SuperRequiresProviderCLIExitTwoOracle(t *testing.T) {
	found := []string{}
	for _, item := range cliProbes() {
		if item.Name != "super_api_key_requires_provider" && item.Name != "short_super_api_key_requires_provider" {
			continue
		}
		if !item.Rejects || len(item.Arguments) < 4 {
			t.Fatalf("super missing-provider oracle became a success path: %+v", item)
		}
		found = append(found, item.Name)
		reference := commandResult{ExitCode: 2, Output: "required arguments: --provider"}
		if diffs := compareProbe(item, reference, reference); len(diffs) != 0 {
			t.Fatalf("canonical reject contract no longer passes: %v", diffs)
		}
		for _, exit := range []int{0, 1, 130} {
			different := commandResult{ExitCode: exit, Output: "--api-key requires --provider", Accepted: exit == 0}
			if diffs := compareProbe(item, reference, different); len(diffs) == 0 {
				t.Fatalf("wrong provider-usage exit %d escaped CLI oracle", exit)
			}
		}
	}
	for _, name := range []string{"super_api_key_requires_provider", "short_super_api_key_requires_provider"} {
		if !slices.Contains(found, name) {
			t.Fatalf("missing tagged provider usage oracle %s", name)
		}
	}
}
