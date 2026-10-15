package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// A worker-per-scenario mock, private fixture root and deterministic report
// order are part of the oracle: sharing mutable server plans across workers
// can make two independently wrong product responses appear to match.
func TestProdex043713DeepSeekParallelScenariosKeepIsolatedMocksAndOrderedResults(t *testing.T) {
	started := make(chan int, 4)
	gates := [4]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	defer func() {
		for _, gate := range gates {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
	}()
	var mu sync.Mutex
	seen := map[string]int{}
	specs := make([]differentialScenario043713, 0, 4)
	for i := range 4 {
		specs = append(specs, differentialScenario043713{
			name: string(rune('a' + i)),
			run: func(mock *mockServer) (scenarioResult, error) {
				mu.Lock()
				seen[mock.baseURL()]++
				mu.Unlock()
				started <- i
				<-gates[i]
				return scenarioResult{Name: string(rune('a' + i))}, nil
			},
		})
	}
	type outcome struct {
		results []scenarioResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := executeDifferentialScenarios043713(specs, "all", 3)
		done <- outcome{result, err}
	}()
	waitStart := func() int {
		select {
		case i := <-started:
			return i
		case <-time.After(5 * time.Second):
			t.Fatal("isolated mock workers did not enter concurrently")
			return -1
		}
	}
	active := map[int]bool{}
	for range 3 {
		active[waitStart()] = true
	}
	if len(active) != 3 {
		t.Fatalf("parallel executor reused a worker slot: %v", active)
	}
	for i := range active {
		close(gates[i])
		break
	}
	active[waitStart()] = true
	if len(active) != 4 {
		t.Fatalf("fourth isolated fixture never began: %v", active)
	}
	for i := range gates {
		select {
		case <-gates[i]:
		default:
			close(gates[i])
		}
	}
	select {
	case got := <-done:
		if got.err != nil || len(got.results) != 4 {
			t.Fatalf("parallel results error=%v results=%+v", got.err, got.results)
		}
		for i, result := range got.results {
			if result.Name != string(rune('a'+i)) {
				t.Fatalf("report ordering changed: %+v", got.results)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parallel executor deadlocked")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 {
		t.Fatalf("scenarios did not use four different loopback mock servers: %v", seen)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal("a scenario reused another scenario's mock")
		}
	}
}

func TestProdex043713DeepSeekParallelFailuresNeverReportPartialPass(t *testing.T) {
	specs := []differentialScenario043713{
		{name: "first", run: func(*mockServer) (scenarioResult, error) { return scenarioResult{Name: "first"}, nil }},
		{name: "broken", run: func(*mockServer) (scenarioResult, error) { return scenarioResult{}, errors.New("synthetic failure") }},
		{name: "last", run: func(*mockServer) (scenarioResult, error) { return scenarioResult{Name: "last"}, nil }},
	}
	for _, workers := range []int{1, 3} {
		result, err := executeDifferentialScenarios043713(specs, "all", workers)
		if len(result) != 0 || err == nil || !strings.Contains(err.Error(), "broken scenario") {
			t.Fatalf("workers=%d published partial result=%+v err=%v", workers, result, err)
		}
	}
	for _, tc := range []struct {
		name    string
		workers int
	}{
		{name: "unknown", workers: 1}, {name: "first", workers: 0}, {name: "first", workers: 5}, {name: "first", workers: 3},
	} {
		result, err := executeDifferentialScenarios043713(specs, tc.name, tc.workers)
		if err == nil || len(result) != 0 {
			t.Fatalf("invalid worker selection allowed: %+v %v", result, err)
		}
	}
	single, err := executeDifferentialScenarios043713(specs, "first", 1)
	if err != nil || len(single) != 1 || single[0].Name != "first" {
		t.Fatalf("single scenario parity changed: %+v %v", single, err)
	}
}
