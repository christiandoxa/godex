package gemini

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProdex04384GeminiNonobjectToolProjectionEmitsNoFrame(t *testing.T) {
	for _, fixture := range []struct {
		name, calls string
		pending     bool
	}{
		{"null entry", `[null]`, true},
		{"bool entry", `[false]`, true},
		{"number entry", `[0]`, true},
		{"string entry", `[""]`, true},
		{"array entry", `[[]]`, true},
		{"empty calls", `[]`, false},
		{"null calls", `null`, false},
		{"object calls", `{}`, false},
		{"bool calls", `false`, false},
		{"string calls", `"ignored"`, false},
		{"object entry", `[{}]`, false},
		{"absent", "", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			fields := `"content":"before"`
			if fixture.calls != "" {
				fields += `,"tool_calls":` + fixture.calls
			}
			var wire bytes.Buffer
			err := state.accept(&wire, []byte(`{"choices":[{"delta":{`+fields+`}}]}`))
			if fixture.pending {
				if err == nil || wire.Len() != 0 {
					t.Fatalf("unsupported projection emitted bytes or advanced normally: error=%v bytes=%d", err, wire.Len())
				}
				return
			}
			if err != nil || !strings.Contains(wire.String(), `"delta":"before"`) {
				t.Fatalf("projection control lost preceding content: %v", err)
			}
		})
	}
}

func TestGeminiPendingProjectionStopsOnRequestOrBodyCancellation(t *testing.T) {
	for _, closeBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "request canceled", true: "body closed"}[closeBody], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source := &geminiBodyCloseProbe04384{
				packet:  strings.NewReader("data: " + `{"choices":[{"delta":{"tool_calls":[null]}}]}` + "\n\ndata: [DONE]\n\n"),
				started: make(chan struct{}), closed: make(chan struct{}),
			}
			response := translateGeminiChatCompatibleStream043711(ctx, &http.Response{StatusCode: 200, Header: make(http.Header), Body: source}, nil, nil)
			defer response.Body.Close()
			type readResult struct {
				body []byte
				err  error
			}
			finished := make(chan readResult, 1)
			go func() {
				body, err := io.ReadAll(response.Body)
				finished <- readResult{body, err}
			}()
			<-source.started
			if closeBody {
				if err := response.Body.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case result := <-finished:
				if len(result.body) != 0 || result.err == nil || !closeBody && !errors.Is(result.err, context.Canceled) {
					t.Fatalf("pending projection fabricated output or lost cancellation: bytes=%d error=%v", len(result.body), result.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("pending projection did not stop on cancellation")
			}
			response.Body.Close()
			if source.closes.Load() != 1 {
				t.Fatal("pending projection closed upstream more than once")
			}
		})
	}
}

func TestGeminiEmbeddedErrorPrecedesNonobjectToolProjection(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
	var wire bytes.Buffer
	err := state.accept(&wire, []byte(`{"error":{"message":"synthetic failure"},"choices":[{"delta":{"tool_calls":[null]}}]}`))
	if err != nil || strings.Count(wire.String(), "event: response.failed") != 1 || strings.Contains(wire.String(), "response.completed") {
		t.Fatalf("nonobject tool hid an embedded provider error: %v", err)
	}
}

func TestGeminiPendingToolProjectionPreservesPreviouslyEmittedFrames(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"content":"previous"}}]}`)); err != nil {
		t.Fatal(err)
	}
	previous := wire.String()
	err := state.accept(&wire, []byte(`{"choices":[{"delta":{"content":"must-not-commit","tool_calls":[null]}}]}`))
	if err == nil || wire.String() != previous {
		t.Fatal("pending projection dropped prior bytes or committed its current frame")
	}
}
