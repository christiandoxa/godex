//go:build linux || darwin

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestProdex04383RealtimeSocketProbeMeasuresRawIdleRead(t *testing.T) {
	var output bytes.Buffer
	if err := realtimeSocketMode04383(&output); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"measured=true", "listener_nonblocking=true", "accepted_nonblocking=", "idle_read=would_block", "read_wait_us="} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("missing socket diagnostic %q: %s", field, output.String())
		}
	}
}
