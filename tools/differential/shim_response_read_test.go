package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCodexShimDistinguishesReadFailureFromBodyOverflow(t *testing.T) {
	for _, fixture := range []struct {
		name, body, diagnostic string
		declared               int
		wantExit               int
	}{
		{"truncated", "synthetic-ok", "synthetic Codex shim response read failed: unexpected EOF\n", 100, 1},
		{"oversized", strings.Repeat("x", bodyLimit+1), "synthetic Codex shim response exceeded its bound\n", bodyLimit + 1, 1},
		{"oversized truncated", strings.Repeat("x", bodyLimit+1), "synthetic Codex shim response exceeded its bound\n", bodyLimit + 10, 1},
		{"complete", "synthetic-ok", "", len("synthetic-ok"), 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", strconv.Itoa(fixture.declared))
				_, _ = io.WriteString(w, fixture.body)
			}))
			defer server.Close()
			resultPath := filepath.Join(t.TempDir(), "child-exchange.json")
			t.Setenv("DIFFERENTIAL_CHILD_RESULT", resultPath)
			t.Setenv("DIFFERENTIAL_CONTINUATION_PHASE", "")
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
			if err != nil || exit != fixture.wantExit || string(diagnostic) != fixture.diagnostic {
				t.Fatalf("exit=%d diagnostic=%q error=%v", exit, diagnostic, err)
			}
			if exit != 0 {
				if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
					t.Fatalf("failed capture wrote a valid-looking exchange: %v", err)
				}
				return
			}
			result, err := readExchange(resultPath)
			if err != nil || result.Body != fixture.body || result.Status != http.StatusOK {
				t.Fatalf("complete response changed: status=%d error=%v", result.Status, err)
			}
		})
	}
}
