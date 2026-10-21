package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiSSESeparateJSONDataLines(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{true: "native", false: "chat"}[native], func(t *testing.T) {
			first := `{"choices":[{"delta":{"content":"first"}}]}`
			next := `{"choices":[{"delta":{"content":"second"}}]}`
			if native {
				first = `{"candidates":[{"content":{"parts":[{"text":"first"}]}}]}`
				next = `{"candidates":[{"content":{"parts":[{"text":"second"}]}}]}`
			}
			wire := "data: " + first + "\ndata: " + next + "\n\ndata: [DONE]\n\n"
			upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
			var response *proxymodel.Response
			if native {
				response = translateGeminiNativeStream(upstream, nil)
			} else {
				response = translateGeminiChatCompatibleStream043711(upstream)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			var text strings.Builder
			completed := 0
			for _, frame := range append(decoder.Feed(body), decoder.Finish()...) {
				var event map[string]any
				if err := json.Unmarshal(frame, &event); err != nil {
					t.Fatal(err)
				}
				if event["type"] == "response.output_text.delta" {
					text.WriteString(event["delta"].(string))
				}
				if event["type"] == "response.completed" {
					completed++
				}
			}
			if text.String() != "firstsecond" || completed != 1 || strings.Contains(string(body), "event: response.failed") {
				t.Fatalf("separate data values lost text/completion: %s", body)
			}
		})
	}
}

func TestProdex04384GeminiSSEJoinedValuesPrecedeSeparateParsing(t *testing.T) {
	for _, native := range []bool{false, true} {
		name, chunk := "chat", "{\"choices\":\n[{\"delta\":{\"content\":\"joined-control\"}}]}"
		if native {
			name, chunk = "native", "{\"candidates\":\n[{\"content\":{\"parts\":[{\"text\":\"joined-control\"}]}}]}"
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			var err error
			if native {
				state := &geminiStreamState{responseID: "resp-fixture"}
				err = state.consume(&output, []byte(chunk), nil)
			} else {
				state := &geminiChatCompatibleStream043711{id: "resp-fixture", created: 1}
				err = state.accept(&output, []byte(chunk))
			}
			if err != nil || !strings.Contains(output.String(), `"delta":"joined-control"`) {
				t.Errorf("valid joined JSON was parsed as separate lines: %s err=%v", output.String(), err)
			}
		})
	}
}

func TestProdex04384GeminiSSESeparateValuesValidateBeforeEmission(t *testing.T) {
	for _, native := range []bool{false, true} {
		first := `{"choices":[{"delta":{"content":"must-not-emit"}}]}`
		if native {
			first = `{"candidates":[{"content":{"parts":[{"text":"must-not-emit"}]}}]}`
		}
		for _, invalid := range []string{`{"partial":`, `{"unused":"\ud800"}`, `{"unused":1e309}`} {
			var output bytes.Buffer
			var err error
			if native {
				state := &geminiStreamState{responseID: "resp-fixture"}
				err = state.consume(&output, []byte(first+"\n"+invalid), nil)
			} else {
				state := &geminiChatCompatibleStream043711{id: "resp-fixture", created: 1}
				err = state.accept(&output, []byte(first+"\n"+invalid))
			}
			if err == nil || output.Len() != 0 {
				t.Errorf("invalid second JSON line emitted first candidate: native=%v invalid=%q err=%v output=%s", native, invalid, err, output.String())
			}
		}
	}
}

func TestProdex04384GeminiSSESeparateValuesSkipBlankLines(t *testing.T) {
	values, valid := geminiSeparateSSEValues04384([]byte("  null\n\n {\"a\":1} \n"))
	if !valid || len(values) != 2 || string(values[0]) != "null" || string(values[1]) != `{"a":1}` {
		t.Errorf("separate-value whitespace changed: %q valid=%v", values, valid)
	}
}

func TestProdex04384GeminiSSEDoneLinePrecedesOtherValues(t *testing.T) {
	for _, native := range []bool{false, true} {
		visible := `{"choices":[{"delta":{"content":"visible-control"}}]}`
		if native {
			visible = `{"candidates":[{"content":{"parts":[{"text":"visible-control"}]}}]}`
		}
		for _, frame := range []string{`{"error":"must-not-fail"}` + "\n[DONE]", "[DONE]\n" + visible, visible + "\n [DONE] \ninvalid-json"} {
			var output bytes.Buffer
			var err error
			if native {
				state := &geminiStreamState{responseID: "resp-fixture"}
				if err = state.consume(&output, []byte(visible), nil); err == nil {
					err = state.consume(&output, []byte(frame), nil)
				}
			} else {
				state := &geminiChatCompatibleStream043711{id: "resp-fixture", created: 1}
				if err = state.accept(&output, []byte(visible)); err == nil {
					err = state.accept(&output, []byte(frame))
				}
			}
			if err != nil || strings.Count(output.String(), `"delta":"visible-control"`) != 1 || strings.Count(output.String(), "event: response.completed") != 1 || strings.Contains(output.String(), "event: response.failed") {
				t.Errorf("DONE marker lost event precedence: native=%v err=%v frame=%q output=%s", native, err, frame, output.String())
			}
		}
	}
}
