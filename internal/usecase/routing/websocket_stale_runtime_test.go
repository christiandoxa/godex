package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestWebSocketKnownOwnerPreviousResponseFailsStaleWithoutRetryReason(t *testing.T) {
	failurePayload := []byte(`{"type":"response.failed","status":400,"response":{"id":"resp-missing","error":{"code":"previous_response_not_found","message":"missing"}}}`)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-owner"),
		websocketPreviousFailureTestResponse(failurePayload, ""),
	}}
	var waits []time.Duration
	router := newWebSocketPreviousResponseRuntimeRouter(t, gateway, &waits)

	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 61))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	continuation, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","previous_response_id":"resp-owner"}`, 61),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	body, err := io.ReadAll(continuation.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		t.Fatalf("stale payload is not JSON: %q", body)
	}
	responseObject, _ := payload["response"].(map[string]any)
	errorObject, _ := responseObject["error"].(map[string]any)
	if continuation.Result.AccountID != "account-a" ||
		continuation.Result.Response.StatusCode != http.StatusConflict ||
		len(waits) != 0 ||
		strings.Join(gateway.accounts, ",") != "account-a,account-a" ||
		websocketJSONText(payload["type"]) != "response.failed" ||
		websocketJSONText(errorObject["code"]) != "stale_continuation" ||
		strings.Contains(string(body), "previous_response_not_found") {
		t.Fatalf("stale result = account=%q status=%d waits=%v accounts=%v body=%s",
			continuation.Result.AccountID, continuation.Result.Response.StatusCode, waits, gateway.accounts, body)
	}
}

func TestWebSocketSemanticLockedContinuationRetriesOwnerOnTaggedSchedule(t *testing.T) {
	failure := []byte(`{"type":"response.failed","status":400,"error":{"code":"previous_response_not_found","message":"missing"}}`)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-owner"),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketCommittedTestResponse("resp-recovered"),
	}}
	var waits []time.Duration
	router := newWebSocketPreviousResponseRuntimeRouter(t, gateway, &waits)

	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 62))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	body := `{"type":"response.create","previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`
	recovered, err := router.Forward(t.Context(), websocketDispatchRequest(body, 62))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	wantWaits := []time.Duration{75 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}
	if recovered.Result.AccountID != "account-a" ||
		recovered.Result.Response.WebSocketResponseID != "resp-recovered" ||
		!reflect.DeepEqual(waits, wantWaits) ||
		strings.Join(gateway.accounts, ",") != "account-a,account-a,account-a,account-a,account-a" {
		t.Fatalf("recovery result = account=%q response=%q waits=%v accounts=%v",
			recovered.Result.AccountID, recovered.Result.Response.WebSocketResponseID, waits, gateway.accounts)
	}
}

func TestWebSocketPreviousResponseTurnStateRetryHasNoArbitraryFourKiBCap(t *testing.T) {
	const turnStateBytes = 5 << 10
	turnState := strings.Repeat("t", turnStateBytes)
	failure := []byte(`{"type":"response.failed","status":400,"error":{"code":"previous_response_not_found","message":"missing"}}`)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-owner"),
		websocketPreviousFailureTestResponse(failure, turnState),
		websocketCommittedTestResponse("resp-next"),
	}}
	var waits []time.Duration
	router := newWebSocketPreviousResponseRuntimeRouter(t, gateway, &waits)

	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 63))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := router.Forward(
		t.Context(),
		websocketDispatchRequest(`{"type":"response.create","previous_response_id":"resp-owner"}`, 63),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if len(gateway.requests) != 3 {
		t.Fatalf("turn-state retry requests = %d, want 3; waits=%v", len(gateway.requests), waits)
	}
	if !reflect.DeepEqual(waits, []time.Duration{75 * time.Millisecond}) ||
		gateway.requests[2].Header.Get("x-codex-turn-state") != turnState ||
		!gateway.requests[2].WebSocketPolicy.TurnStateOverride {
		t.Fatalf("turn-state retry = waits=%v requests=%d state-bytes=%d override=%t",
			waits, len(gateway.requests), len(gateway.requests[2].Header.Get("x-codex-turn-state")),
			gateway.requests[2].WebSocketPolicy.TurnStateOverride)
	}
}

func TestStaleWebSocketPayloadPreservesTaggedResponseFailedShapes(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		nested   bool
		wantType string
	}{
		{
			name:   "nested response error",
			input:  `{"type":"response.failed","response":{"error":{"code":"previous_response_not_found"}}}`,
			nested: true, wantType: "response.failed",
		},
		{
			name:     "top-level response error",
			input:    `{"type":"response.failed","error":{"code":"previous_response_not_found"}}`,
			wantType: "response.failed",
		},
		{
			name:     "plain text fallback",
			input:    "previous_response_not_found: missing",
			wantType: "error",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := translateStaleWebSocketPayload([]byte(test.input))
			var payload map[string]any
			if err := json.Unmarshal(output, &payload); err != nil {
				t.Fatal(err)
			}
			var errorObject map[string]any
			if test.nested {
				responseObject, _ := payload["response"].(map[string]any)
				errorObject, _ = responseObject["error"].(map[string]any)
			} else {
				errorObject, _ = payload["error"].(map[string]any)
			}
			if websocketJSONText(payload["type"]) != test.wantType ||
				int(payload["status"].(float64)) != http.StatusConflict ||
				websocketJSONText(errorObject["code"]) != "stale_continuation" ||
				websocketJSONText(errorObject["message"]) != websocketStaleContinuationMessage ||
				strings.Contains(string(output), "previous_response_not_found") {
				t.Fatalf("translated payload = %s", output)
			}
		})
	}
}

func TestProdex043712StaleWebSocketPayloadForwardsWithoutPreviousResponseMarker(t *testing.T) {
	for _, payload := range []string{
		`{"type":"response.failed","status":400,"error":{"code":"invalid_request_error","message":"bad input"}}`,
		`{"type":"error","status":503,"error":{"code":"server_is_overloaded"}}`,
		`{"type":"error","error":{"message":"previous_response_not_found: unrelated text"}}`,
		"upstream text that is not a previous-response error",
	} {
		got := translateStaleWebSocketPayload([]byte(payload))
		if string(got) != payload {
			t.Fatalf("unrelated stale candidate changed payload: got %q, want %q", got, payload)
		}
	}
	invalidUTF8 := []byte{0xff, 'p', 'r', 'e', 'v', 'i', 'o', 'u', 's', '_', 'r', 'e', 's', 'p', 'o', 'n', 's', 'e', '_', 'n', 'o', 't', '_', 'f', 'o', 'u', 'n', 'd'}
	if got := translateStaleWebSocketPayload(invalidUTF8); len(got) == 0 || got[0] != 0xff {
		t.Fatalf("invalid UTF-8 payload was rewritten: %q", got)
	}
}

func TestProdex043712StaleWebSocketPayloadRecognizesInvalidPreviousIDMarker(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`." + `"}}`)
	got := translateStaleWebSocketPayload(payload)
	var event map[string]any
	if err := json.Unmarshal(got, &event); err != nil {
		t.Fatal(err)
	}
	if websocketJSONText(event["type"]) != "error" || int(event["status"].(float64)) != http.StatusConflict {
		t.Fatalf("invalid previous ID was not translated: %s", got)
	}
}

func TestProdex043712StaleWebSocketPayloadRecognizesStructuredToolContextMarker(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"no tool call found for output"}}`)
	got := translateStaleWebSocketPayload(payload)
	if !strings.Contains(string(got), `"code":"stale_continuation"`) {
		t.Fatalf("structured tool-context marker was not translated: %s", got)
	}
}

func newWebSocketPreviousResponseRuntimeRouter(
	t *testing.T,
	gateway *websocketDispatchGateway,
	waits *[]time.Duration,
) *Router {
	t.Helper()
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: "account-a",
		Wait: func(ctx context.Context, delay time.Duration) error {
			*waits = append(*waits, delay)
			return ctx.Err()
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func websocketCommittedTestResponse(responseID string) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(strings.NewReader("committed-frame")),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: responseID,
	}
}

func websocketPreviousFailureTestResponse(payload []byte, turnState string) *proxymodel.Response {
	var frames bytes.Buffer
	if err := websocketframe.WriteFrame(&frames, 1, payload, false); err != nil {
		panic(err)
	}
	return &proxymodel.Response{
		StatusCode:         http.StatusOK,
		Header:             make(http.Header),
		Body:               io.NopCloser(bytes.NewReader(frames.Bytes())),
		WebSocketFrames:    true,
		WebSocketTurnState: turnState,
		PrecommitFailure:   &proxymodel.PrecommitFailure{Code: "previous_response_not_found"},
	}
}

func TestProdex04379BoundToolOutputMissingChainUsesGenericStaleError(t *testing.T) {
	failure := []byte(`{"type":"response.failed","status":400,"response":{"error":{"code":"previous_response_not_found","message":"missing"}}}`)
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-tool-owner"),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketPreviousFailureTestResponse(failure, ""),
		websocketPreviousFailureTestResponse(failure, ""),
	}}
	var waits []time.Duration
	router := newWebSocketPreviousResponseRuntimeRouter(t, gateway, &waits)
	first, err := router.Forward(t.Context(), websocketDispatchRequest(`{"type":"response.create"}`, 439))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	const tool = `{"type":"response.create","previous_response_id":"resp-tool-owner","input":[{"type":"function_call_output","call_id":"call-locked","output":"done"}]}`
	continuation, err := router.Forward(t.Context(), websocketDispatchRequest(tool, 439))
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Close()
	body, err := io.ReadAll(continuation.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
		Error  struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &event) != nil || event.Type != "error" ||
		event.Status != http.StatusConflict || event.Error.Code != "stale_continuation" {
		t.Fatalf("locked tool failure did not use native generic stale frame: %s", body)
	}
	if !reflect.DeepEqual(waits, []time.Duration{75 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}) ||
		len(gateway.requests) != 5 || len(gateway.accounts) != 5 {
		t.Fatalf("locked affinity retries = %v requests=%d owners=%v", waits, len(gateway.requests), gateway.accounts)
	}
	for index := 1; index < len(gateway.requests); index++ {
		if gateway.accounts[index] != "account-a" ||
			string(gateway.requests[index].Body) != tool {
			t.Fatalf("attempt %d changed owner or dropped tool context: owner=%s request=%s",
				index, gateway.accounts[index], gateway.requests[index].Body)
		}
	}
}
