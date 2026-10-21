package proxy

import (
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

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

// Tagged RuntimeSseTapState clears every known response ID after a committed
// `previous_response_not_found`, not just the failed event's response ID.
// The source request's previous ID and any earlier prefix ID are dead; a
// separate account's durable bindings must remain untouched.
func TestProdex04383CommittedSSEDeadResponseClearsOwnChain(t *testing.T) {
	const mainID = websocketHardOwner04379
	const otherID = websocketSoftOwner04379
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	otherHome := t.TempDir()
	if err := os.Chmod(otherHome, 0o700); err != nil {
		t.Fatal(err)
	}
	store := routingrepo.NewStore(t.TempDir())
	key := func(id string) string {
		digest := sha256.Sum256([]byte("previous:" + id))
		return hex.EncodeToString(digest[:])
	}
	now := time.Now().Unix()
	var bindings []routingentity.Binding
	for _, tc := range []struct{ id, owner string }{{"resp-main", mainID}, {"resp-independent-other", otherID}} {
		bindings = append(bindings, routingentity.Binding{Kind: "previous", Key: key(tc.id), AccountID: tc.owner, UpdatedUnix: now})
	}
	if _, err := store.Merge(t.Context(), bindings); err != nil {
		t.Fatal(err)
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{Gateway: &sseLineageWarmupGateway04383{},
		Bindings: store, PreferredAccount: mainID,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: mainID, Home: home, Enabled: true}, {ID: otherID, Home: otherHome, Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	warmup, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: []byte(`{"model":"gpt-6","input":[]}`), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
	if err != nil {
		t.Fatal(err)
	}
	_ = warmup.Close()
	header := http.Header{"Content-Type": {"text/event-stream"}}
	prefix := "data: " + `{"type":"response.output_text.delta","delta":"first","response":{"id":"resp-stream-prefix","turn_state":"stream-turn"}}` + "\n\n"
	rest := "data: " + `{"type":"response.failed","response":{"id":"resp-stream-failed","error":{"code":"previous_response_not_found"}}}` + "\n\n"
	if err := router.Observe(t.Context(), mainID, header, []byte(prefix), true, "openai"); err != nil {
		t.Fatal(err)
	}
	proxy := &Proxy{router: router, maxInspect: 64 << 10}
	output := httptest.NewRecorder()
	if !proxy.forwardStream(t.Context(), output, io.NopCloser(strings.NewReader(rest)), []byte(prefix), mainID, header, "openai", "resp-main") {
		t.Fatal("committed SSE stream aborted")
	}
	if output.Body.String() != prefix+rest {
		t.Fatal("committed SSE original wire bytes changed")
	}
	stored, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The tagged release planner also drops the now-unreferenced direct
	// turn owner after its last response/turn pairing has disappeared.
	turnDigest := sha256.Sum256([]byte("turn:stream-turn"))
	for _, entry := range stored {
		if entry.Kind == "turn" && entry.Key == hex.EncodeToString(turnDigest[:]) {
			t.Fatalf("dead SSE stream left orphaned turn-state binding with owner %q", entry.AccountID)
		}
	}
	for _, tc := range []struct{ id, owner string }{{"resp-main", ""}, {"resp-stream-prefix", ""}, {"resp-stream-failed", ""}, {"resp-independent-other", otherID}} {
		found := ""
		for _, entry := range stored {
			if entry.Kind == "previous" && entry.Key == key(tc.id) {
				found = entry.AccountID
			}
		}
		if found != tc.owner {
			t.Fatalf("committed previous error id=%s remained owned by %q want %q", tc.id, found, tc.owner)
		}
		if tc.owner == "" && tc.id != "resp-main" {
			turn, _, err := store.LoadResponseTurnState(t.Context(), home, key(tc.id), time.Now())
			if err != nil || turn != "" {
				t.Fatalf("dead stream response %s retained private turn=%q err=%v", tc.id, turn, err)
			}
		}
	}
}
