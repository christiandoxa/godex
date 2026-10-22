package gemini

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384GeminiSSEAccumulatesJSONAcrossEvents(t *testing.T) {
	for _, native := range []bool{false, true} {
		name, first, last := "chat", `{"choices":[{"delta":{"content":"frag`, `ment"}}]}`
		if native {
			name, first, last = "native", `{"candidates":[{"content":{"parts":[{"text":"frag`, `ment"}]}}]}`
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			var accept func([]byte) error
			if native {
				state := &geminiStreamState{responseID: "resp-fragment"}
				accept = func(data []byte) error { return state.consume(&output, data, nil) }
			} else {
				state := &geminiChatCompatibleStream043711{id: "resp-fragment", created: 1}
				accept = func(data []byte) error { return state.accept(&output, data) }
			}
			if err := accept([]byte(first)); err != nil || output.Len() != 0 {
				t.Fatalf("partial JSON failed/emitted before reassembly: err=%v output=%s", err, output.String())
			}
			if err := accept([]byte(last)); err != nil || strings.Count(output.String(), `"delta":"fragment"`) != 1 {
				t.Fatalf("complete reassembly did not emit exactly one text delta: err=%v output=%s", err, output.String())
			}
			if err := accept([]byte("[DONE]")); err != nil || strings.Count(output.String(), "event: response.completed") != 1 || strings.Contains(output.String(), "event: response.failed") {
				t.Errorf("reassembled stream failed completion: err=%v output=%s", err, output.String())
			}
		})
	}
}

func TestProdex04384GeminiSSEIncompleteJSONFailsAtDoneAndEOF(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, done := range []bool{false, true} {
			wire := "data: " + `{"partial":` + "\n\n"
			if done {
				wire += "data: [DONE]\n\n"
			}
			upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
			var body io.ReadCloser
			if native {
				body = translateGeminiNativeStream(upstream, nil).Body
			} else {
				body = translateGeminiChatCompatibleStream043711(upstream, nil).Body
			}
			output, err := io.ReadAll(body)
			body.Close()
			if err != nil || strings.Count(string(output), "event: response.failed") != 1 || !bytes.Contains(output, []byte(`"message":"incomplete JSON segment"`)) || bytes.Contains(output, []byte("event: response.completed")) || bytes.Contains(output, []byte("event: response.created")) {
				t.Errorf("incomplete fragment native=%v DONE=%v err=%v output=%s", native, done, err, output)
			}
		}
	}
}

func TestProdex04384GeminiSSECompleteValuesDoNotConsumePendingFragments(t *testing.T) {
	state := &geminiChatCompatibleStream043711{id: "resp-fixture", created: 1}
	var output bytes.Buffer
	for _, chunk := range []string{`{"choices":[{"delta":{"content":"frag`, `{"choices":[{"delta":{"content":"standalone"}}]}`, `ment"}}]}`, `[DONE]`} {
		if err := state.accept(&output, []byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	wire := output.String()
	if strings.Count(wire, `"delta":"standalone"`) != 1 || strings.Count(wire, `"delta":"fragment"`) != 1 || strings.Index(wire, `"delta":"standalone"`) > strings.Index(wire, `"delta":"fragment"`) || strings.Contains(wire, "event: response.failed") {
		t.Errorf("standalone complete packet consumed/reordered pending fragment: %s", wire)
	}
}
