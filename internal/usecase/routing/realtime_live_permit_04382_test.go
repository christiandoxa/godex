package routing

import (
	"bytes"
	"context"
	"fmt"
	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex 0.438.2's tagged `websocket_realtime_connection_retains_its_live_duplex_permit`
// says live audio/video duplex differs from an idle completed Responses
// socket. The former owns its weighted permit until its session is CLOSED;
// a competing Responses request must not dispatch to a saturated profile.
type realtimePermitGateway04382 struct {
	mu    sync.Mutex
	calls int
	peers []net.Conn
}

func (gateway *realtimePermitGateway04382) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("unexpected regular HTTP routing in realtime fixture")
}

func (gateway *realtimePermitGateway04382) ExecuteWebSocketMessage(
	_ context.Context, request proxymodel.Request, account proxymodel.Account,
) (*proxymodel.Response, error) {
	if account.ID != "synthetic-ready" || !request.WebSocketMessage {
		return nil, fmt.Errorf("unexpected realtime fixture owner/request: %q, %#v", account.ID, request)
	}
	client, peer := net.Pipe()
	gateway.mu.Lock()
	gateway.peers = append(gateway.peers, peer)
	gateway.calls++
	gateway.mu.Unlock()
	return &proxymodel.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header:     make(http.Header), Body: client,
		WebSocketFrames: true, WebSocketRealtimeDuplex: request.WebSocketPolicy.RealtimeDuplex,
		FirstEventCommitted: true,
	}, nil
}

func (gateway *realtimePermitGateway04382) CloseWebSocketSession(uint64) {}

func (gateway *realtimePermitGateway04382) Close() {
	gateway.mu.Lock()
	peers := append([]net.Conn(nil), gateway.peers...)
	gateway.mu.Unlock()
	for _, peer := range peers {
		_ = peer.Close()
	}
}

func TestProdex04382RealtimeDuplexRetainsWeightedPermitUntilClose(t *testing.T) {
	gateway := &realtimePermitGateway04382{}
	defer gateway.Close()
	waitEntered := make(chan struct{}, 1)
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-ready", Home: "synthetic-home", Enabled: true}}, nil
		},
		ProfileInflightWait: func(ctx context.Context, changed <-chan struct{}, _ time.Duration) error {
			select {
			case waitEntered <- struct{}{}:
			default:
			}
			select {
			case <-changed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	realtime := proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/godex/live", Header: make(http.Header),
		Body: []byte(`{"type":"session.update"}`), WebSocketMessage: true,
		WebSocketPolicy: proxymodel.WebSocketPolicy{RealtimeDuplex: true},
	}
	realtime.Header.Set("Upgrade", "websocket")
	live, err := router.Forward(context.Background(), realtime)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	assertCount := func(want int) {
		t.Helper()
		router.mu.Lock()
		got := router.inflight["synthetic-ready"]
		router.mu.Unlock()
		if got != want {
			t.Fatalf("inflight weight=%d, want %d", got, want)
		}
	}
	assertCount(2)
	if !live.Result.Response.WebSocketRealtimeDuplex || live.Result.Response.Body == nil {
		t.Fatal("realtime duplex was incorrectly treated as idle Responses")
	}
	// Reading an actual upstream realtime *data* frame is still active
	// duplex, never a Responses completion. It must not release capacity.
	gateway.mu.Lock()
	peer := gateway.peers[0]
	gateway.mu.Unlock()
	framePayload := []byte(`{"type":"output_audio.delta","delta":"synthetic"}`)
	written := make(chan error, 1)
	go func() { written <- websocketframe.WriteFrame(peer, 1, framePayload, false) }()
	receivedHeader, err := websocketframe.ReadHeader(live.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	receivedPayload, err := receivedHeader.ReadPayload(live.Result.Response.Body, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if receivedHeader.Opcode != 1 || !bytes.Equal(receivedPayload, framePayload) {
		t.Fatalf("realtime upstream data changed: opcode=%d payload=%q", receivedHeader.Opcode, receivedPayload)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	assertCount(2)
	follower := realtime
	follower.Path = "/backend-api/godex/responses"
	follower.WebSocketPolicy = proxymodel.WebSocketPolicy{}
	follower.Body = []byte(`{"type":"response.create"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type result struct {
		exchange *Exchange
		err      error
	}
	received := make(chan result, 1)
	go func() { exchange, err := router.Forward(ctx, follower); received <- result{exchange, err} }()
	select {
	case <-waitEntered:
		assertCount(2)
		gateway.mu.Lock()
		calls := gateway.calls
		gateway.mu.Unlock()
		if calls != 1 {
			t.Fatalf("realtime live permit leaked and dispatched follower: %d calls", calls)
		}
	case got := <-received:
		if got.exchange != nil {
			_ = got.exchange.Close()
		}
		t.Fatalf("competing Responses turn bypassed active realtime permit: %v", got.err)
	case <-ctx.Done():
		t.Fatal("saturated followup failed to enter permit wait")
	}
	// Even after the first realtime event, no EOF means the live permit is
	// still held. Only closing the duplex releases the profile capacity.
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		if got.err != nil {
			t.Fatalf("follower was stranded after live close: %v", got.err)
		}
		assertCount(2)
		if err := got.exchange.Close(); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("realtime close did not wake waiting request")
	}
	assertCount(0)
	router.mu.Lock()
	admissions := router.profileInflightAdmissionsTotal
	releases := router.profileInflightReleasesTotal
	underflows := router.profileInflightReleaseUnderflowsTotal
	router.mu.Unlock()
	if admissions != 2 || releases != 2 || underflows != 0 {
		t.Fatalf("realtime permit lifecycle admission/release/underflow=%d/%d/%d", admissions, releases, underflows)
	}
}
