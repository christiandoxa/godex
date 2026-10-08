package routing

import (
	"fmt"
	"testing"
	"time"
)

// Prodex 0.435.9 guarantees that releasing capacity publishes the new
// generation before a waiting selector can observe the freed counter.
// Godex represents that generation by replacing/closing a channel under
// the same mutex as the in-flight map.
func TestProdex04359CapacityReleasePublishesGenerationAtomically(t *testing.T) {
	const rounds = 500
	router := &Router{
		inflight:                 make(map[string]int),
		inflightChanged:          make(chan struct{}),
		profileInflightHardLimit: 1,
	}
	request := profileInflightRequest("/v1/chat/completions")
	for i := 0; i < rounds; i++ {
		release, ok := router.tryAcquireProfileInflight("main", request, false)
		if !ok {
			t.Fatalf("round %d acquire rejected", i)
		}
		router.mu.Lock()
		held := router.inflight["main"]
		observedGeneration := router.inflightChanged
		router.mu.Unlock()
		if held != 1 {
			t.Fatalf("round %d held=%d", i, held)
		}
		outcome := make(chan error, 1)
		go func() {
			select {
			case <-observedGeneration:
				router.mu.Lock()
				after := router.inflight["main"]
				nextGeneration := router.inflightChanged
				router.mu.Unlock()
				if after != 0 || nextGeneration == observedGeneration {
					outcome <- fmt.Errorf("released permit=%d, new generation=%t", after, nextGeneration != observedGeneration)
				} else {
					outcome <- nil
				}
			case <-time.After(time.Second):
				outcome <- fmt.Errorf("waiter did not observe released capacity")
			}
		}()
		release()
		if err := <-outcome; err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
	}
	router.mu.Lock()
	admissions, releases, underflows := router.profileInflightAdmissionsTotal, router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal
	router.mu.Unlock()
	if admissions != rounds || releases != rounds || underflows != 0 {
		t.Fatalf("metrics: admissions=%d releases=%d underflows=%d", admissions, releases, underflows)
	}
}
