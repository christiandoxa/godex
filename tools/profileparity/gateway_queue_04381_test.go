package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestProdex04381GatewayQueueOracleRejectsMutations(t *testing.T) {
	canonical := queueGatewayResult04381{
		status: []int{http.StatusOK, http.StatusOK, http.StatusOK}, calls: 3,
		queued: true, fresh: true, authenticated: true, exited: true,
	}
	if !sameGatewayQueue04381(canonical, canonical) {
		t.Fatal("canonical admission queue result rejected")
	}
	mutations := []func(*queueGatewayResult04381){
		func(result *queueGatewayResult04381) { result.calls++ },
		func(result *queueGatewayResult04381) { result.status[1] = http.StatusGatewayTimeout },
		func(result *queueGatewayResult04381) { result.queued = false },
		func(result *queueGatewayResult04381) { result.fresh = false },
		func(result *queueGatewayResult04381) { result.authenticated = false },
		func(result *queueGatewayResult04381) { result.exited = false },
	}
	for index, mutate := range mutations {
		candidate := canonical
		candidate.status = append([]int(nil), canonical.status...)
		mutate(&candidate)
		if sameGatewayQueue04381(canonical, candidate) || sameGatewayQueue04381(candidate, canonical) {
			t.Fatalf("mutation %d escaped admission queue oracle: %#v", index, candidate)
		}
	}
}

func TestProdex04381GatewayQueueRealExecutables(t *testing.T) {
	prodex := strings.TrimSpace(os.Getenv("GODEX_TEST_PRODEX_04381_BIN"))
	godex := strings.TrimSpace(os.Getenv("GODEX_TEST_GODEX_04381_BIN"))
	if prodex == "" || godex == "" {
		t.Skip("set exact source-verified binary paths to run admission queue fixture")
	}
	stages, err := checkGatewayQueue04381("", cliOptions{prodex: prodex, godex: godex})
	if err != nil || len(stages) != 1 || stages[0].Requests != 3 {
		t.Fatalf("real admission queue parity diverged: stages=%+v err=%v", stages, err)
	}
}
