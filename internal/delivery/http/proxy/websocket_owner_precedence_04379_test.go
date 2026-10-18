package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

const (
	websocketHardOwner04379 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	websocketSoftOwner04379 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type websocketOwnerPriorityGateway04379 struct {
	mu     sync.Mutex
	owners []string
	routes []quotamodel.RouteKind
}

func (*websocketOwnerPriorityGateway04379) Execute(
	context.Context, proxymodel.Request, proxymodel.Account,
) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("unexpected standard HTTP forwarding")
}

func (gateway *websocketOwnerPriorityGateway04379) ExecuteWebSocketMessage(
	_ context.Context, request proxymodel.Request, account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.owners = append(gateway.owners, account.ID)
	gateway.routes = append(gateway.routes, request.QuotaSelection.RouteKind)
	gateway.mu.Unlock()
	id := "resp-main-resumed"
	if account.ID == websocketSoftOwner04379 {
		id = "resp-session-b"
	}
	var framed bytes.Buffer
	_ = websocketframe.WriteFrame(&framed, 1,
		[]byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"%s"}}`, id)), false)
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body:            io.NopCloser(bytes.NewReader(framed.Bytes())),
		WebSocketFrames: true, FirstEventCommitted: true, WebSocketResponseID: id,
	}, nil
}

func (*websocketOwnerPriorityGateway04379) CloseWebSocketSession(uint64) {}

func TestProdex04379PublicWebSocketHardPreviousOutranksConflictingSession(t *testing.T) {
	store := routingrepo.NewStore(t.TempDir())
	key := func(kind, text string) string {
		hash := sha256.Sum256([]byte(kind + ":" + text))
		return hex.EncodeToString(hash[:])
	}
	now := time.Now().Unix()
	if _, err := store.Merge(t.Context(), []routingentity.Binding{
		{Kind: "previous", Key: key("previous", "resp-main"), AccountID: websocketHardOwner04379, UpdatedUnix: now},
		{Kind: "turn", Key: key("turn", "turn-a"), AccountID: websocketHardOwner04379, UpdatedUnix: now},
		{Kind: "session", Key: key("session", "sess-conflict"), AccountID: websocketSoftOwner04379, UpdatedUnix: now},
	}); err != nil {
		t.Fatal(err)
	}
	gateway := &websocketOwnerPriorityGateway04379{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, Bindings: store,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: websocketHardOwner04379, Home: "/synthetic/a", Enabled: true},
				{ID: websocketSoftOwner04379, Home: "/synthetic/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	socket, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
	defer socket.Close()
	if status, _ := readResponsesPublicHandshake(t, reader); status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake returned %d", status)
	}
	send := func(payload string) string {
		t.Helper()
		// Each turn persists ownership; a previous turn must not spend its I/O budget.
		if err := socket.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := socket.Write(protocolClientFrame(1, true, []byte(payload), true)); err != nil {
			t.Fatal(err)
		}
		return readResponsesPublicTextFrame(t, reader)
	}
	first := send(`{"type":"response.create","previous_response_id":"resp-main","session_id":"sess-conflict","input":[]}`)
	if !strings.Contains(first, "resp-main-resumed") {
		t.Fatalf("trusted previous-response owner did not outrank soft session: %s", first)
	}
	unknown := send(`{"type":"response.create","previous_response_id":"resp-unknown","session_id":"sess-conflict","input":[]}`)
	if !strings.Contains(unknown, "resp-main-resumed") {
		t.Fatalf("unbound previous-response discovery was blocked before upstream: %s", unknown)
	}
	for _, variant := range []string{
		`{"type":"response.create","x-codex-turn-state":"turn-a","session_id":"sess-conflict","input":[]}`,
		`{"type":"response.create","client_metadata":{"x-codex-turn-state":"turn-a"},"session_id":"sess-conflict","input":[]}`,
	} {
		if response := send(variant); !strings.Contains(response, "resp-main-resumed") {
			t.Fatalf("trusted turn-state owner did not outrank soft session: %s", response)
		}
	}
	unknownTurn := send(`{"type":"response.create","x-codex-turn-state":"turn-missing","session_id":"sess-conflict","input":[]}`)
	if !strings.Contains(unknownTurn, "resp-main-resumed") &&
		!strings.Contains(unknownTurn, "resp-session-b") {
		t.Fatalf("unbound turn-state discovery was blocked before upstream: %s", unknownTurn)
	}
	last := send(`{"type":"response.create","session_id":"sess-conflict","input":[]}`)
	if !strings.Contains(last, "resp-session-b") {
		t.Fatalf("session-only continuation lost its rightful owner: %s", last)
	}
	gateway.mu.Lock()
	owners := append([]string(nil), gateway.owners...)
	routes := append([]quotamodel.RouteKind(nil), gateway.routes...)
	gateway.mu.Unlock()
	if len(owners) != 6 ||
		owners[0] != websocketHardOwner04379 ||
		owners[1] != websocketHardOwner04379 ||
		owners[2] != websocketHardOwner04379 ||
		owners[3] != websocketHardOwner04379 ||
		(owners[4] != websocketHardOwner04379 && owners[4] != websocketSoftOwner04379) ||
		owners[5] != websocketSoftOwner04379 {
		t.Fatalf("upstream attempt owners=%v, want hard owners on A, one eligible turn-state discovery, then soft session B", owners)
	}
	for _, route := range routes {
		if route != quotamodel.RouteKindWebSocket {
			t.Fatalf("public response WebSocket routed with %q selection, want websocket", route)
		}
	}
	bindings, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	originalSessionOwner := ""
	for _, binding := range bindings {
		if binding.Key == key("session", "sess-conflict") {
			originalSessionOwner = binding.AccountID
		}
	}
	if originalSessionOwner != websocketSoftOwner04379 {
		t.Fatalf("soft session ownership overwritten by harder predecessor: %s", originalSessionOwner)
	}
}
