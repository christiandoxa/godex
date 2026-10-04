package openai

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	prodex04353WebSocketLookaheadBytes    = 8 << 10
	prodex04353WebSocketHardAffinityBytes = 512 << 10
)

func TestWebSocketMessagePolicyMatchesProdex04353(t *testing.T) {
	fresh := proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}
	for _, test := range []struct {
		name   string
		policy proxymodel.WebSocketPolicy
		reused bool
		header string
		hold   bool
		retry  bool
	}{
		{name: "fresh", policy: fresh, hold: true, retry: true},
		{name: "request session", policy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true, RequestSession: true}, retry: true},
		{name: "previous response", policy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true, RequestPreviousResponse: true}},
		{name: "request turn state", policy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true, RequestTurnState: true}},
		{name: "turn state override", policy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true, TurnStateOverride: true}},
		{name: "header override fail closed", policy: fresh, header: "turn-a"},
		{name: "reused session", policy: fresh, reused: true},
		{name: "promotion denied", policy: proxymodel.WebSocketPolicy{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := websocketParityRequest("{\"type\":\"response.create\"}", 0, test.policy)
			if test.header != "" {
				request.Header.Set("x-codex-turn-state", test.header)
			}
			plan := websocketResponsePlanFor(request, test.reused)
			if plan.holdPromotionAllowed != test.hold || plan.transportRetryAllowed != test.retry {
				t.Fatalf("plan = %#v, want hold=%t retry=%t", plan, test.hold, test.retry)
			}
		})
	}
}

func TestWebSocketMessageConstantsMatchProdex04353(t *testing.T) {
	if websocketPrecommitLookaheadBytes != 8<<10 ||
		websocketPrecommitHardAffinityBytes != 512<<10 ||
		websocketPrecommitProgressTimeout != 8*time.Second ||
		websocketCommittedStreamIdleTimeout != 300*time.Second ||
		websocketUpstreamMaxFrameBytes != 16<<20 ||
		websocketUpstreamMaxMessageBytes != 64<<20 {
		t.Fatalf(
			"websocket constants = lookahead %d hard %d precommit %s idle %s frame %d message %d",
			websocketPrecommitLookaheadBytes,
			websocketPrecommitHardAffinityBytes,
			websocketPrecommitProgressTimeout,
			websocketCommittedStreamIdleTimeout,
			websocketUpstreamMaxFrameBytes,
			websocketUpstreamMaxMessageBytes,
		)
	}
}

func TestFreshWebSocketMessagePromotesAtEightKiBLookahead(t *testing.T) {
	firstSent := make(chan struct{})
	finish := make(chan struct{})
	first := []byte("{\"type\":\"response.in_progress\",\"padding\":\"" +
		strings.Repeat("x", prodex04353WebSocketLookaheadBytes) + "\"}")
	completed := []byte("{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lookahead\"}}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := writeWebSocketParityEvent(connection, first); err != nil {
			t.Errorf("write hold: %v", err)
			return
		}
		close(firstSent)
		<-finish
		_ = writeWebSocketParityEvent(connection, completed)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)

	result := make(chan struct {
		response *proxymodel.Response
		err      error
	}, 1)
	go func() {
		response, err := transport.ExecuteWebSocketMessage(
			context.Background(),
			websocketParityRequest("{\"type\":\"response.create\"}", 1, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
			proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
		)
		result <- struct {
			response *proxymodel.Response
			err      error
		}{response: response, err: err}
	}()
	<-firstSent
	var outcome struct {
		response *proxymodel.Response
		err      error
	}
	select {
	case outcome = <-result:
	case <-time.After(time.Second):
		close(finish)
		t.Fatal("fresh precommit hold did not promote after 8 KiB")
	}
	if outcome.err != nil {
		close(finish)
		t.Fatal(outcome.err)
	}
	if !outcome.response.FirstEventCommitted {
		close(finish)
		t.Fatal("8 KiB fresh hold remained uncommitted")
	}
	close(finish)
	body, err := io.ReadAll(outcome.response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = outcome.response.Body.Close()
	if want := websocketParityFrames(t, first, completed); !bytes.Equal(body, want) {
		t.Fatalf("promoted frames differ: got %d want %d", len(body), len(want))
	}
}

func TestHardAffinityWebSocketHoldFailsClosedAt512KiB(t *testing.T) {
	oversized := []byte("{\"type\":\"response.in_progress\",\"padding\":\"" +
		strings.Repeat("x", prodex04353WebSocketHardAffinityBytes) + "\"}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, oversized)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	_, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest(
			"{\"type\":\"response.create\",\"previous_response_id\":\"resp_owner\"}",
			2,
			proxymodel.WebSocketPolicy{RequestPreviousResponse: true},
		),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err == nil || !strings.Contains(err.Error(), "bounded hard-affinity limit") {
		t.Fatalf("hard-affinity precommit error = %v", err)
	}
}

func TestPrecommitFailureDoesNotExposeHeldMetadata(t *testing.T) {
	hold := []byte("{\"type\":\"response.created\",\"response\":{\"id\":\"resp_hold\"}}")
	failure := []byte("{\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\"}}")
	releaseFailure := make(chan struct{})
	holdSent := make(chan struct{})
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, hold)
		close(holdSent)
		<-releaseFailure
		_ = writeWebSocketParityEvent(connection, failure)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)

	done := make(chan struct {
		response *proxymodel.Response
		err      error
	}, 1)
	go func() {
		response, err := transport.ExecuteWebSocketMessage(
			context.Background(),
			websocketParityRequest("{\"type\":\"response.create\"}", 3, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
			proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
		)
		done <- struct {
			response *proxymodel.Response
			err      error
		}{response, err}
	}()
	<-holdSent
	select {
	case early := <-done:
		close(releaseFailure)
		if early.response != nil {
			_ = early.response.Body.Close()
		}
		t.Fatal("response.created promoted before lookahead budget")
	default:
	}
	close(releaseFailure)
	outcome := <-done
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	defer outcome.response.Body.Close()
	if outcome.response.FirstEventCommitted ||
		outcome.response.PrecommitFailure == nil ||
		outcome.response.PrecommitFailure.Code != "rate_limit_exceeded" {
		t.Fatalf("precommit failure = %#v", outcome.response)
	}
	body, err := io.ReadAll(outcome.response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := websocketParityFrames(t, failure); !bytes.Equal(body, want) {
		t.Fatalf("uncommitted hold leaked: got %d want %d", len(body), len(want))
	}
}

func TestPrecommitTransportFailureDoesNotExposeHeldFrames(t *testing.T) {
	hold := []byte("{\"type\":\"response.created\",\"response\":{\"id\":\"resp_hold\"}}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, hold)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest("{\"type\":\"response.create\"}", 4, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.PrecommitFailure == nil || !response.PrecommitFailure.Transport || response.FirstEventCommitted {
		t.Fatalf("transport failure = %#v", response)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("transport failure exposed %d uncommitted bytes", len(body))
	}
}

func TestLargeTurnStateAndCompletedSessionAreReusable(t *testing.T) {
	turnState := strings.Repeat("turn-", 1600)
	var handshakes atomic.Int32
	server := newWebSocketMessageParityServer(t, http.Header{"X-Codex-Turn-State": {turnState}}, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		handshakes.Add(1)
		for index := 0; index < 2; index++ {
			if _, err := readWebSocketParityRequest(reader); err != nil {
				t.Errorf("read request %d: %v", index, err)
				return
			}
			payload := []byte("{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_done\"}}")
			if err := writeWebSocketParityEvent(connection, payload); err != nil {
				t.Errorf("write response: %v", err)
				return
			}
		}
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
	for index := 0; index < 2; index++ {
		response, err := transport.ExecuteWebSocketMessage(
			context.Background(),
			websocketParityRequest("{\"type\":\"response.create\"}", 55, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
			account,
		)
		if err != nil {
			t.Fatal(err)
		}
		if response.WebSocketTurnState != turnState {
			t.Fatalf("turn state length = %d, want %d", len(response.WebSocketTurnState), len(turnState))
		}
		_, _ = io.Copy(io.Discard, response.Body)
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if handshakes.Load() != 1 {
		t.Fatalf("upstream handshakes = %d, want 1", handshakes.Load())
	}
}

func TestCommittedIncompleteResetsUpstreamSession(t *testing.T) {
	var handshakes atomic.Int32
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		handshakes.Add(1)
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, []byte("{\"type\":\"response.output_text.delta\",\"delta\":\"x\"}"))
		_ = writeWebSocketParityEvent(connection, []byte("{\"type\":\"response.incomplete\"}"))
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest("{\"type\":\"response.create\"}", 66, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	transport.websocketMessageMu.Lock()
	_, retained := transport.websocketMessageSessions[66]
	transport.websocketMessageMu.Unlock()
	if retained {
		t.Fatal("response.incomplete connection was recycled")
	}
	if handshakes.Load() != 1 {
		t.Fatalf("handshakes = %d", handshakes.Load())
	}
}
