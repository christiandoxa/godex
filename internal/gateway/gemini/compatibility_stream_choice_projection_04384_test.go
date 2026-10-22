package gemini

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestProdex04384GeminiChatSSEChoiceProjectionPreservesPendingFrame(t *testing.T) {
	for _, fixture := range []struct {
		choices string
		pending bool
	}{
		{`[null,{}]`, true},
		{`[false,{}]`, true},
		{`[0,{}]`, true},
		{`["",{}]`, true},
		{`[[],{}]`, true},
		{`[{},null,false,0,"",[]]`, false},
		{`[]`, false},
		{`null`, false},
		{`false`, false},
		{`{}`, false},
	} {
		t.Run(fixture.choices, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			err := state.accept(&wire, []byte(`{"choices":`+fixture.choices+`}`))
			var pending geminiChatProjectionPending04384
			if fixture.pending {
				if !errors.As(err, &pending) || wire.Len() != 0 || state.started {
					t.Fatal("nonobject first choice committed a frame or lost its pending reader")
				}
			} else if err != nil || !strings.HasPrefix(wire.String(), "event: response.created\r\n") {
				t.Fatal("ignored choices or non-first entries blocked a valid frame")
			}
		})
	}
}

func TestProdex04384GeminiChatSSEEmbeddedErrorPrecedesChoiceProjection(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"error":"synthetic error","choices":[null]}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wire.String(), "event: response.failed\r\n") || !state.done || state.started {
		t.Fatal("choice projection displaced embedded-error priority")
	}
}
