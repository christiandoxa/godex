package main

import "testing"

func TestProviderIdentityOracleRejectsMatchingFailuresAndUnsafeLaunches(t *testing.T) {
	accepted := providerIdentityProbe04390{provider: "openai-compatible"}
	launched := providerIdentityRun04390{
		stdout: "context-probe-ok\n", started: true, homeMatch: true,
		arguments: []string{"exec", "resume", nativeSession04390, "continue"},
	}
	rejected := providerIdentityProbe04390{provider: "synthetic-unknown", reject: true}
	failed := providerIdentityRun04390{exit: 1, diagnostic: "resumed session has an unsupported provider identity"}
	for _, fixture := range []struct {
		probe providerIdentityProbe04390
		run   providerIdentityRun04390
	}{
		{accepted, launched}, {rejected, failed},
	} {
		if err := verifyProviderIdentityRun04390(fixture.probe, fixture.run); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*providerIdentityRun04390){
		func(run *providerIdentityRun04390) { run.exit = 2 },
		func(run *providerIdentityRun04390) { run.stdout = "changed" },
		func(run *providerIdentityRun04390) { run.started = false },
		func(run *providerIdentityRun04390) { run.homeMatch = false },
		func(run *providerIdentityRun04390) {
			run.arguments = []string{"exec", "fork", nativeSession04390, "continue"}
		},
	} {
		changed := launched
		mutate(&changed)
		if err := verifyProviderIdentityRun04390(accepted, changed); err == nil {
			t.Fatal("accepted an incorrect native provider launch")
		}
	}
	for _, mutate := range []func(*providerIdentityRun04390){
		func(run *providerIdentityRun04390) { run.exit = 0 },
		func(run *providerIdentityRun04390) { run.stdout = "unexpected output" },
		func(run *providerIdentityRun04390) { run.started = true },
		func(run *providerIdentityRun04390) { run.diagnostic = "unrelated failure" },
		func(run *providerIdentityRun04390) { run.diagnostic += ": " + rejected.provider },
	} {
		changed := failed
		mutate(&changed)
		if err := verifyProviderIdentityRun04390(rejected, changed); err == nil {
			t.Fatal("accepted an unsafe or unrelated provider rejection")
		}
	}
}
