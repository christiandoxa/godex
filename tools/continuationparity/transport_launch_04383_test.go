package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("GODEX_TEST_MANAGED_LAUNCH_FAILURE") == "1" {
		fmt.Fprintln(os.Stderr, "synthetic startup failed: Authorization: Bearer private-fixture-value")
		os.Exit(23)
	}
	os.Exit(m.Run())
}

func TestProdex04383ManagedLaunchFailurePreservesSafeChildDiagnostic(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := product{binary: binary, user: t.TempDir(), env: []string{"GODEX_TEST_MANAGED_LAUNCH_FAILURE=1"}}
	endpoint, stop, err := startTransportLaunch04383(p, "http://127.0.0.1:1/backend-api")
	if stop != nil {
		defer stop()
	}
	if err == nil || endpoint != "" || !strings.Contains(err.Error(), "exit status 23") || !strings.Contains(err.Error(), "synthetic startup failed") || !strings.Contains(err.Error(), "<redacted>") || strings.Contains(err.Error(), "private-fixture-value") {
		t.Fatal("failed managed launch lost its safe child diagnostic or reported success")
	}
}

func TestProdex04383ManagedLaunchDiagnosticsAreBoundedAndRedacted(t *testing.T) {
	var output transportLaunchDiagnostic04383
	parts := []string{"synthetic launch failed\nAuthorization: Bear", "er private-fixture-value\nrefresh_token=private-refresh-value\n", strings.Repeat("x", 16<<10), "out-of-bounds"}
	for _, part := range parts {
		if count, err := io.WriteString(&output, part); err != nil || count != len(part) {
			t.Fatal("diagnostic capture did not drain child stderr")
		}
	}
	message := output.String()
	if len(output) != 8<<10 || !strings.Contains(message, "synthetic launch failed") || !strings.Contains(message, "<redacted>") || strings.Contains(message, "private-fixture-value") || strings.Contains(message, "private-refresh-value") || strings.Contains(message, "out-of-bounds") {
		t.Fatal("managed launch diagnostic lost context, exceeded its bound or exposed credentials")
	}
}
