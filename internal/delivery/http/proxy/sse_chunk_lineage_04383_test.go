package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

// Tagged 0.438.3 SSE tap owns one state machine across precommit prefix and
// later HTTP body reads. Unmarked later response IDs inherit its active
// turn; a late first turn backfills prefix IDs, while new turns never mutate
// the existing ID/turn pair. Original client SSE bytes remain unchanged.
func TestProdex04383ProxySSEPreservesTurnLineageAcrossChunks(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, rest string
		turns              map[string]string
	}{
		{"inherit-from-prefix",
			"data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"state-prefix"},"delta":"ok"}` + "\n\n",
			"data: " + `{"type":"response.completed","response":{"id":"resp-later"}}` + "\n\n",
			map[string]string{"resp-prefix": "state-prefix", "resp-later": "state-prefix"},
		},
		{"backfill-late-turn",
			"data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix"},"delta":"ok"}` + "\n\n",
			"data: " + `{"type":"response.completed","response":{"id":"resp-later","turn_state":"state-later"}}` + "\n\n",
			map[string]string{"resp-prefix": "state-later", "resp-later": "state-later"},
		},
		{"separate-states",
			"data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"state-prefix"},"delta":"ok"}` + "\n\n",
			"data: " + `{"type":"response.completed","response":{"id":"resp-later","turn_state":"state-later"}}` + "\n\n",
			map[string]string{"resp-prefix": "state-prefix", "resp-later": "state-later"},
		},
		{"large-postcommit-event",
			"data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"state-prefix"},"delta":"ok"}` + "\n\n",
			"data: " + `{"type":"response.completed","response":{"id":"resp-later"},"padding":"` + strings.Repeat("x", 72<<10) + `"}` + "\n\n",
			map[string]string{"resp-prefix": "state-prefix", "resp-later": "state-prefix"},
		},
		{"partial-precommit-line",
			"data: " + `{"type":"response.output_text.delta","response":{"id":"resp-prefix","turn_state":"state-prefix"},`,
			`"delta":"ok"}` + "\n\n" + "data: " + `{"type":"response.completed","response":{"id":"resp-later"}}` + "\n\n",
			map[string]string{"resp-prefix": "state-prefix", "resp-later": "state-prefix"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.Chmod(home, 0o700); err != nil {
				t.Fatal(err)
			}
			store := routingrepo.NewStore(t.TempDir())
			const account = websocketHardOwner04379
			router, err := routingusecase.NewRouter(routingusecase.Config{
				Bindings: store,
				Gateway:  &sseLineageWarmupGateway04383{},
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: account, Home: home, Enabled: true}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			// The real router populates managed account homes on its first
			// dispatch. This synthetic warmup gives the observer the same
			// private profile-home before the later streamed SSE response.
			warmup, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body:           []byte(`{"model":"gpt-6","input":[]}`),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = warmup.Close()
			headers := http.Header{"Content-Type": {"text/event-stream"}}
			// BindSuccessfulResponse handles complete prefix events before
			// the committed stream is delivered to the HTTP bridge.
			if err := router.Observe(t.Context(), account, headers, []byte(tc.prefix), true, "openai"); err != nil {
				t.Fatal(err)
			}
			proxy := &Proxy{router: router, maxInspect: 64 << 10}
			output := httptest.NewRecorder()
			if !proxy.forwardStream(t.Context(), output, io.NopCloser(strings.NewReader(tc.rest)), []byte(tc.prefix), account, headers, "openai") {
				t.Fatal("committed HTTP stream unexpectedly aborted")
			}
			if !bytes.Equal(output.Body.Bytes(), []byte(tc.prefix+tc.rest)) {
				t.Fatalf("SSE wire altered across prefix/body boundary: %q", output.Body.Bytes())
			}
			bindings, err := store.Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for id, turn := range tc.turns {
				hash := sha256.Sum256([]byte("previous:" + id))
				key := hex.EncodeToString(hash[:])
				found := false
				for _, binding := range bindings {
					if binding.Kind == "previous" && binding.Key == key && binding.AccountID == account {
						found = true
					}
				}
				if !found {
					t.Fatalf("durable previous response %s missing", id)
				}
				got, _, err := store.LoadResponseTurnState(t.Context(), home, key, time.Now())
				if err != nil || got != turn {
					t.Fatalf("durable sidecar response=%s got=%q want=%q err=%v", id, got, turn, err)
				}
			}
		})
	}
}

// Used only to initialize the real managed account-home map before emitting
// streaming response events. It does not mint any tested response ID.
type sseLineageWarmupGateway04383 struct{}

func (*sseLineageWarmupGateway04383) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"object":"response","output":[]}`))}, nil
}
