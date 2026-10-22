package gemini

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestProdex04384GeminiChatSSEWholeFrameProjectionRetainsPending(t *testing.T) {
	for _, packet := range []string{"null", "false", "0", `""`, "[]", "{}"} {
		t.Run(packet, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			err := state.accept(&wire, []byte(packet))
			var pending geminiChatProjectionPending04384
			if packet == "{}" {
				if err != nil || !strings.HasPrefix(wire.String(), "event: response.created\r\n") {
					t.Fatal("empty object lost its valid projected frame")
				}
			} else if !errors.As(err, &pending) || wire.Len() != 0 || state.started {
				t.Fatal("nonobject whole frame emitted an event or lost its pending reader")
			}
		})
	}
}
