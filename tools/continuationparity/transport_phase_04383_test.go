package main

import (
	"reflect"
	"testing"
)

// The scoped oracles must cover the exact same published 0.439.0 finite
// fixture in the same order. In particular, the original deadline and busy
// permit case may not vanish from the default or native mode.
func TestProdex04383WebSocketTransportSlicesExhaustive(t *testing.T) {
	want := []string{
		"full-history", "unbound-history", "quota-history", "unbound-quota-history",
		"turn-missing", "delta-only", "unbound-delta", "unbound-quota-delta",
		"previous-response", "unknown-previous", "previous-quota", "unknown-quota",
		"previous-rate", "unknown-rate", "previous-overload", "unknown-overload",
		"postcommit", "cancellation", "deadline", "busy-history",
	}
	all := websocketTransportCases04383("all")
	core := websocketTransportCases04383("core")
	lifecycle := websocketTransportCases04383("lifecycle")
	capacity := websocketTransportCases04383("capacity")
	joined := append(append(append([]string(nil), core...), lifecycle...), capacity...)
	if !reflect.DeepEqual(all, want) || len(core) != 17 || len(lifecycle) != 2 || len(capacity) != 1 ||
		!reflect.DeepEqual(joined, all) ||
		len(websocketTransportCases04383("unsupported")) != 0 {
		t.Fatalf("transport fixture lost/moved cases: all=%v core=%v lifecycle=%v capacity=%v", all, core, lifecycle, capacity)
	}
	seen := make(map[string]bool, len(all))
	for _, name := range all {
		if seen[name] {
			t.Fatalf("transport fixture repeated a case: %s", name)
		}
		seen[name] = true
	}
	for _, name := range []string{"cancellation", "deadline", "busy-history"} {
		if !seen[name] {
			t.Fatalf("transport lifecycle case absent: %s", name)
		}
	}
}
