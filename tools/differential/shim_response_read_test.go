package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCodexShimDistinguishesReadFailureFromBodyOverflow(t *testing.T) {
	for _, fixture := range []struct {
		name, body, diagnostic, retainedError string
		declared, wantExit                    int
		capture                               bool
	}{
		{name: "truncated", body: "synthetic-ok", diagnostic: "synthetic Codex shim response read failed: unexpected EOF\n", declared: 100, wantExit: 1},
		{name: "oversized", body: strings.Repeat("x", bodyLimit+1), diagnostic: "synthetic Codex shim response exceeded its bound\n", declared: bodyLimit + 1, wantExit: 1},
		{name: "oversized truncated", body: strings.Repeat("x", bodyLimit+1), diagnostic: "synthetic Codex shim response exceeded its bound\n", declared: bodyLimit + 10, wantExit: 1},
		{name: "complete", body: "synthetic-ok", declared: len("synthetic-ok")},
		{name: "captured truncated", body: "synthetic-ok", diagnostic: "synthetic Codex shim response read failed: unexpected EOF\n", retainedError: "unexpected EOF", declared: 100, wantExit: 1, capture: true},
		{name: "captured oversized", body: strings.Repeat("x", bodyLimit+1), diagnostic: "synthetic Codex shim response exceeded its bound\n", declared: bodyLimit + 1, wantExit: 1, capture: true},
		{name: "captured complete", body: "synthetic-ok", declared: len("synthetic-ok"), capture: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Length", strconv.Itoa(fixture.declared))
				_, _ = io.WriteString(w, fixture.body)
			}))
			defer server.Close()
			resultPath := filepath.Join(t.TempDir(), "child-exchange.json")
			t.Setenv("DIFFERENTIAL_CHILD_RESULT", resultPath)
			t.Setenv("DIFFERENTIAL_CONTINUATION_PHASE", "")
			t.Setenv("DIFFERENTIAL_CAPTURE_READ_ERROR", "")
			if fixture.capture {
				t.Setenv("DIFFERENTIAL_CAPTURE_READ_ERROR", "1")
			}
			if fixture.retainedError != "" {
				t.Setenv("DIFFERENTIAL_CONTINUATION_PHASE", "same-process")
			}
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			original := os.Stderr
			exit := func() int {
				os.Stderr = write
				defer func() { os.Stderr = original }()
				return runCodexShim([]string{"-c", "model_providers.fixture.base_url=" + strconv.Quote(server.URL)})
			}()
			_ = write.Close()
			diagnostic, err := io.ReadAll(read)
			if err != nil || exit != fixture.wantExit || string(diagnostic) != fixture.diagnostic || calls.Load() != 1 {
				t.Fatalf("exit=%d diagnostic=%q error=%v", exit, diagnostic, err)
			}
			if exit != 0 && fixture.retainedError == "" {
				if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
					t.Fatalf("failed capture wrote a valid-looking exchange: %v", err)
				}
				return
			}
			result, err := readExchange(resultPath)
			if err != nil || result.Body != fixture.body || result.Status != http.StatusOK {
				t.Fatalf("complete response changed: status=%d error=%v", result.Status, err)
			}
			data, err := os.ReadFile(resultPath)
			var record struct {
				ReadError string          `json:"read_error"`
				Seed      json.RawMessage `json:"successful_seed"`
			}
			if err != nil || json.Unmarshal(data, &record) != nil || record.ReadError != fixture.retainedError || len(record.Seed) != 0 {
				t.Fatal("read failure was lost or treated as a successful seed")
			}
		})
	}
}
