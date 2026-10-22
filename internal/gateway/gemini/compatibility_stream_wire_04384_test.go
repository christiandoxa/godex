package gemini

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384GeminiAPIKeySSEPreservesLiteralSerdeCharacters(t *testing.T) {
	for _, text := range []string{"plain-control", "<>&/🌍\u2028\u2029", `literal \u2028 \u003c`} {
		t.Run(text, func(t *testing.T) {
			chunk := `{"choices":[{"delta":{"content":` + jsonLiteral04384(t, text) + `}}]}`
			response := translateGeminiChatCompatibleStream043711(&http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n"))}, nil, nil)
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			wire := string(data)
			if text == "<>&/🌍\u2028\u2029" && strings.Count(wire, text) != 3 {
				t.Errorf("API-key SSE escaped literal serde characters in delta/done/final output: %q", wire)
			}
			if text == `literal \u2028 \u003c` && strings.Count(wire, `literal \\u2028 \\u003c`) != 3 {
				t.Errorf("API-key SSE changed escaped-text lookalikes: %q", wire)
			}
			if text == "plain-control" && strings.Count(wire, text) != 3 {
				t.Errorf("API-key SSE lost plain control: %q", wire)
			}
		})
	}
}

func TestProdex04384GeminiAPIKeySSEKeepsIntegerUsageTokens(t *testing.T) {
	for _, number := range []string{"2", "9007199254740993", "18446744073709551615"} {
		t.Run(number, func(t *testing.T) {
			state := &geminiChatCompatibleStream043711{id: "resp-fixture", created: 1}
			var output bytes.Buffer
			if err := state.accept(&output, []byte(`{"usage":{"prompt_tokens":`+number+`,"completion_tokens":1,"total_tokens":`+number+`}}`)); err != nil {
				t.Fatal(err)
			}
			if err := state.complete(&output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), `"input_tokens":`+number+`,`) || !strings.Contains(output.String(), `"total_tokens":`+number+`}`) {
				t.Errorf("usage integer was converted to a floating-point token: %s", output.String())
			}
		})
	}
}
