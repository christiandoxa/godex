package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const (
	invalidPreviousOwnerA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	invalidPreviousOwnerB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestWebSocketInvalidPreviousResponseSignalsFullContextAndRetainsSessionOwner(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-owner"),
		websocketInvalidPreviousTestResponse(),
		websocketCommittedTestResponse("resp-recovered"),
	}}
	store := routingrepo.NewStore(t.TempDir())
	router := newInvalidPreviousRouter(t, gateway, store)

	first, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","session_id":"session-chain","input":[{"type":"message","role":"user","content":"turn one"}]}`, 91),
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.AccountID != invalidPreviousOwnerA {
		t.Fatalf("first owner = %q", first.Result.AccountID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	signal, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","session_id":"session-chain","previous_response_id":"resp-owner","input":[{"type":"message","role":"user","content":"turn two"}]}`, 91),
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(signal.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := signal.Close(); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("signal JSON: %v body=%s", err, body)
	}
	errorValue, _ := value["error"].(map[string]any)
	if signal.Result.AccountID != invalidPreviousOwnerA ||
		signal.Result.Response.StatusCode != http.StatusBadRequest ||
		websocketJSONText(errorValue["type"]) != "invalid_request_error" ||
		websocketJSONText(errorValue["code"]) != "previous_response_not_found" ||
		websocketJSONText(errorValue["message"]) != "Invalid `previous_response_id`." ||
		strings.Contains(string(body), "stale_continuation") {
		t.Fatalf("full-context signal = account=%q status=%d body=%s",
			signal.Result.AccountID, signal.Result.Response.StatusCode, body)
	}

	previousOwner, err := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-owner"}, router.now())
	if err != nil {
		t.Fatal(err)
	}
	sessionOwner, err := router.affinity.owner(t.Context(), affinityKeys{session: "session-chain"}, router.now())
	if err != nil {
		t.Fatal(err)
	}
	if previousOwner != "" || sessionOwner != invalidPreviousOwnerA {
		t.Fatalf("owners after invalid id = previous=%q session=%q", previousOwner, sessionOwner)
	}
	persisted, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range persisted {
		if binding.Kind == "previous" {
			t.Fatalf("dead previous binding persisted: %#v", binding)
		}
	}

	replay, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","session_id":"session-chain","input":[{"type":"message","role":"user","content":"turn one"},{"type":"message","role":"assistant","content":"turn one result"},{"type":"message","role":"user","content":"turn two"}]}`, 92),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if replay.Result.AccountID != invalidPreviousOwnerA ||
		replay.Result.Response.WebSocketResponseID != "resp-recovered" ||
		strings.Join(gateway.accounts, ",") != strings.Join([]string{invalidPreviousOwnerA, invalidPreviousOwnerA, invalidPreviousOwnerA}, ",") {
		t.Fatalf("full-context replay = account=%q response=%q attempts=%v",
			replay.Result.AccountID, replay.Result.Response.WebSocketResponseID, gateway.accounts)
	}
}

func TestWebSocketInvalidPreviousResponseWithoutSessionPassesThrough(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-owner"),
		websocketInvalidPreviousTestResponse(),
	}}
	router := newInvalidPreviousRouter(t, gateway, nil)

	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 93))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","previous_response_id":"resp-owner"}`, 93),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	body, err := io.ReadAll(result.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "Invalid `previous_response_id`.") ||
		strings.Contains(string(body), "previous_response_not_found") ||
		strings.Contains(string(body), "stale_continuation") {
		t.Fatalf("invalid-id pass-through = %s", body)
	}
}

func newInvalidPreviousRouter(
	t *testing.T,
	gateway *websocketDispatchGateway,
	store *routingrepo.Store,
) *Router {
	t.Helper()
	var bindings bindingRepository
	if store != nil {
		bindings = store
	}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: invalidPreviousOwnerA,
		Bindings:         bindings,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: invalidPreviousOwnerA, Home: "/a", Enabled: true},
				{ID: invalidPreviousOwnerB, Home: "/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func websocketInvalidPreviousTestResponse() *proxymodel.Response {
	payload := []byte("{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Invalid `previous_response_id`.\"}}")
	var frames bytes.Buffer
	if err := websocketframe.WriteFrame(&frames, 1, payload, false); err != nil {
		panic(err)
	}
	return &proxymodel.Response{
		StatusCode:      http.StatusOK,
		Header:          make(http.Header),
		Body:            io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code:                      "previous_response_not_found",
			InvalidPreviousResponseID: true,
		},
	}
}
