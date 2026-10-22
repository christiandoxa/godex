package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestProdex04384SameProcessProbeRequiresSuccessfulSeed(t *testing.T) {
	for _, seedStatus := range []int{503, 200} {
		t.Run(strconv.Itoa(seedStatus), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status := seedStatus
				if calls.Add(1) == 2 {
					status = 503
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"id":"chatcmpl-differential"}`)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "child-exchange.json")
			t.Setenv("DIFFERENTIAL_PROVIDER", "gemini")
			t.Setenv("DIFFERENTIAL_CONTINUATION_PHASE", "same-process")
			t.Setenv("DIFFERENTIAL_CHILD_RESULT", path)
			exit := runCodexShim([]string{"-c", "model_providers.fixture.base_url=" + strconv.Quote(server.URL)})
			if seedStatus != 200 {
				if exit != 1 || calls.Load() != 1 {
					t.Fatalf("failed seed masked as a continuation result: exit=%d calls=%d", exit, calls.Load())
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("failed seed wrote a valid-looking exchange: %v", err)
				}
				return
			}
			final, err := readExchange(path)
			if err != nil || exit != 2 || calls.Load() != 2 || final.Status != 503 {
				t.Fatalf("successful seed lost second response: exit=%d calls=%d error=%v", exit, calls.Load(), err)
			}
			var record struct {
				Seed *exchange `json:"successful_seed"`
			}
			data, err := os.ReadFile(path)
			if err != nil || json.Unmarshal(data, &record) != nil || record.Seed == nil ||
				record.Seed.Status != 200 || record.Seed.Body != `{"id":"chatcmpl-differential"}` ||
				record.Seed.Headers.Get("Content-Type") != "application/json" {
				t.Fatal("successful same-process seed exchange was not retained exactly")
			}
		})
	}
}
