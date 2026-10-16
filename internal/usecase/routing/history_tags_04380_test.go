package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestProdex04380FullHistoryRequiresExactProtocolTags(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		input string
		want  bool
	}{
		{"compaction", `[{"type":"compaction"},{"role":"user"}]`, true},
		{"escaped compaction", `[{"type":"compact\u0069on"},{"role":"user"}]`, true},
		{"padded compaction", `[{"type":" compaction "},{"role":"user"}]`, false},
		{"completed turn", `[{"role":"user"},{"role":"assistant"},{"role":"user"}]`, true},
		{"padded first user", `[{"role":" user "},{"role":"assistant"},{"role":"user"}]`, false},
		{"padded assistant", `[{"role":"user"},{"role":"assistant\t"},{"role":"user"}]`, false},
		{"completed function call", `[{"role":"user"},{"type":"function_call"},{"role":"user"}]`, true},
		{"padded function call", `[{"role":"user"},{"type":"function_call "},{"role":"user"}]`, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var input []json.RawMessage
			if err := json.Unmarshal([]byte(fixture.input), &input); err != nil {
				t.Fatal(err)
			}
			if got := reconstructableFullHistory(input); got != fixture.want {
				t.Fatalf("reconstructable = %t, want %t", got, fixture.want)
			}
		})
	}
}

func TestProdex04380InvalidPreviousIDDoesNotReplayPaddedCompaction(t *testing.T) {
	const invalid = `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`
	const original = `{"previous_response_id":"resp-dead","session_id":"session-a","input":[{"type":" compaction "},{"role":"user","content":"continue"}]}`
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{http.StatusBadRequest, "application/json", invalid},
		{http.StatusOK, "application/json", `{"object":"response","id":"must-not-replay"}`},
	}}
	router := newResponseRecoveryRouter(t, gateway, true)
	exchange, err := router.Forward(context.Background(), responsesRecoveryRequest(original))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil || exchange.Result.Response.StatusCode != http.StatusBadRequest || string(body) != invalid || len(gateway.requests) != 1 {
		t.Fatalf("response = status:%d body:%q attempts:%d err:%v", exchange.Result.Response.StatusCode, body, len(gateway.requests), err)
	}
	if string(gateway.requests[0].Body) != original {
		t.Fatal("request changed before forwarding")
	}
}
