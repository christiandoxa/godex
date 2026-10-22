package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiParallelOracleRejectsMatchingControlLeak(t *testing.T) {
	for _, suffix := range []string{"true", "false", "null", "number", "string", "array", "object"} {
		if !json.Valid(geminiParallelInput04384("request-parallel-" + suffix)) {
			t.Fatal("parallel-control fixture is not complete JSON")
		}
	}
	if !geminiParallelNegativeControl04384() {
		t.Fatal("matching leaked parallel controls escaped exact upstream invariants")
	}
}
