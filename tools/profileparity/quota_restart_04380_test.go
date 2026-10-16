package main

import (
	"os"
	"testing"
)

func TestProdex04380QuotaRestartOracleRejectsAuthAndOwnerMutations(t *testing.T) {
	canonical := quotaRestartResult04380{
		firstExit: 0, restartExit: 0,
		first: quotaRestartPhase04380{
			snapshot: true,
			quota: []quotaRestartObservation04380{
				{account: quotaRestartMainAccount04380, path: startupUsagePath, status: 200, authenticated: true},
				{account: quotaRestartSecondAccount04380, path: startupUsagePath, status: 200, authenticated: true},
				{account: quotaRestartThirdAccount04380, path: startupUsagePath, status: 200, authenticated: true},
			},
			responses: []quotaRestartObservation04380{
				{account: quotaRestartMainAccount04380, path: quotaRestartResponsesPath04380, status: 401, authenticated: true},
				{account: quotaRestartSecondAccount04380, path: quotaRestartResponsesPath04380, status: 200, authenticated: true},
			},
		},
		restart: quotaRestartPhase04380{
			snapshot: true,
			quota: []quotaRestartObservation04380{
				{account: quotaRestartMainAccount04380, path: startupUsagePath, status: 200, authenticated: true},
				{account: quotaRestartSecondAccount04380, path: startupUsagePath, status: 200, authenticated: true},
				{account: quotaRestartThirdAccount04380, path: startupUsagePath, status: 200, authenticated: true},
			},
			responses: []quotaRestartObservation04380{
				{account: quotaRestartSecondAccount04380, path: quotaRestartResponsesPath04380, status: 200, authenticated: true},
			},
		},
	}
	if !sameQuotaRestartResult04380(canonical, canonical) {
		t.Fatal("canonical three-profile restart result rejected")
	}

	mutations := []func(*quotaRestartResult04380){
		func(result *quotaRestartResult04380) { result.first.quota[0].authenticated = false },
		func(result *quotaRestartResult04380) {
			result.first.responses[1].account = quotaRestartMainAccount04380
		},
		func(result *quotaRestartResult04380) {
			result.restart.quota = result.restart.quota[:0]
		},
		func(result *quotaRestartResult04380) {
			result.restart.responses = append(result.restart.responses, quotaRestartObservation04380{
				account: quotaRestartMainAccount04380, path: quotaRestartResponsesPath04380, status: 200, authenticated: true,
			})
		},
		func(result *quotaRestartResult04380) {
			for index := range result.restart.quota {
				result.restart.quota[index].account = quotaRestartMainAccount04380
			}
		},
		func(result *quotaRestartResult04380) { result.restart.snapshot = false },
	}
	for index, mutate := range mutations {
		candidate := canonical
		candidate.first.quota = append([]quotaRestartObservation04380(nil), canonical.first.quota...)
		candidate.first.responses = append([]quotaRestartObservation04380(nil), canonical.first.responses...)
		candidate.restart.quota = append([]quotaRestartObservation04380(nil), canonical.restart.quota...)
		candidate.restart.responses = append([]quotaRestartObservation04380(nil), canonical.restart.responses...)
		mutate(&candidate)
		if sameQuotaRestartResult04380(canonical, candidate) {
			t.Fatalf("mutation %d escaped quota/restart oracle: %#v", index, candidate)
		}
	}
}

func TestProdex04380QuotaRestartRealExecutables(t *testing.T) {
	prodex := os.Getenv("GODEX_TEST_PRODEX_04380_BIN")
	godex := os.Getenv("GODEX_TEST_GODEX_04380_BIN")
	if prodex == "" || godex == "" {
		t.Skip("set exact source-verified binary paths to run three-profile quota/restart fixture")
	}
	steps, err := checkQuotaRestart04380(t.TempDir(), cliOptions{prodex: prodex, godex: godex})
	if err != nil || len(steps) != 1 || steps[0].Requests < quotaRestartRequiredRequests04380 || !steps[0].Snapshot {
		t.Fatalf("real three-profile quota/restart parity diverged: steps=%+v err=%v", steps, err)
	}
}
