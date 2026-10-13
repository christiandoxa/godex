package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04379WebSocketLocalWatchdogErrorHasNativeCode(t *testing.T) {
	routeErr := &proxymodel.Error{
		StatusCode: http.StatusGatewayTimeout,
		Message:    "request timed out while waiting for upstream recovery before output started",
	}
	for name, mapper := range map[string]func(error) (int, string, string){
		"responses": responsesWebSocketRouteError,
		"legacy":    websocketRouteError,
	} {
		t.Run(name, func(t *testing.T) {
			status, code, message := mapper(routeErr)
			if status != http.StatusGatewayTimeout || code != "request_timeout" || message != "request timed out" {
				t.Fatalf("native timeout error = %d/%q/%q", status, code, message)
			}
			// A real upstream 504 must not inherit the local watchdog's code.
			status, code, message = mapper(&proxymodel.Error{
				StatusCode: http.StatusGatewayTimeout,
				Message:    "upstream unavailable",
			})
			if status != http.StatusGatewayTimeout || code == "request_timeout" || message != "upstream unavailable" {
				t.Fatalf("foreign 504 incorrectly classified as watchdog = %d/%q/%q", status, code, message)
			}
		})
	}
}

type timeoutPublicGateway04379 struct {
	calls         atomic.Int32
	cancellations atomic.Int32
	alwaysTimeout bool
}

func (*timeoutPublicGateway04379) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, errors.New("HTTP transport must not handle websocket message")
}

func (gateway *timeoutPublicGateway04379) ExecuteWebSocketMessage(
	ctx context.Context, _ proxymodel.Request, _ proxymodel.Account,
) (*proxymodel.Response, error) {
	attempt := gateway.calls.Add(1)
	if attempt == 1 || gateway.alwaysTimeout {
		<-ctx.Done()
		gateway.cancellations.Add(1)
		return nil, ctx.Err()
	}
	var wire bytes.Buffer
	if err := websocketframe.WriteFrame(&wire, 1,
		[]byte(`{"type":"response.completed","response":{"id":"resp-after-timeout"}}`), false); err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(bytes.NewReader(wire.Bytes())),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: "resp-after-timeout",
	}, nil
}

func (*timeoutPublicGateway04379) CloseWebSocketSession(uint64) {}

// The request-scoped watchdog must not close the local WebSocket session or
// poison the profile. Native Codex can handle the error and send another turn.
func TestProdex04379PublicWebSocketTimeoutKeepsSessionAlive(t *testing.T) {
	for _, tc := range []struct {
		name          string
		alwaysTimeout bool
	}{
		{"second request also times out", true},
		{"second request commits output", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &timeoutPublicGateway04379{alwaysTimeout: tc.alwaysTimeout}
			router, err := routingusecase.NewRouter(routingusecase.Config{
				Gateway:                 gateway,
				PrecommitWatchdogBudget: 45 * time.Millisecond,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: "timeout-profile", Home: "/synthetic", Enabled: true}}, nil
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

			connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/prodex/responses")
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
			if status, _ := readResponsesPublicHandshake(t, reader); status != http.StatusSwitchingProtocols {
				t.Fatalf("local websocket upgrade = %d", status)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if _, err := connection.Write(protocolClientFrame(1, true,
					[]byte(`{"type":"response.create","input":[]}`), true)); err != nil {
					t.Fatal(err)
				}
				raw := readResponsesPublicTextFrame(t, reader)
				if attempt == 1 && !tc.alwaysTimeout {
					if !strings.Contains(raw, "resp-after-timeout") {
						t.Fatalf("same websocket did not recover after 504: %s", raw)
					}
					continue
				}
				var payload struct {
					Type   string `json:"type"`
					Status int    `json:"status"`
					Error  struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(raw), &payload); err != nil ||
					payload.Type != "error" || payload.Status != 504 ||
					payload.Error.Code != "request_timeout" || payload.Error.Message != "request timed out" {
					t.Fatalf("native websocket timeout frame = %s, error=%v", raw, err)
				}
			}
			expectedCancelled := int32(1)
			if tc.alwaysTimeout {
				expectedCancelled = 2
			}
			if gateway.calls.Load() != 2 || gateway.cancellations.Load() != expectedCancelled {
				t.Fatalf("attempts/cancellations = %d/%d, want 2/%d",
					gateway.calls.Load(), gateway.cancellations.Load(), expectedCancelled)
			}
		})
	}
}
