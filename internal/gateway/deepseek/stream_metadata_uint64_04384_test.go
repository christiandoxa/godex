package deepseek

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekStreamMetadataKeepsExactIntegerSnapshots(t *testing.T) {
	for _, count := range []string{"7", "9007199254740993", "18446744073709551615"} {
		t.Run(count, func(t *testing.T) {
			packet := `{"id":"chat-presence","model":"deepseek-v4-pro","created":1,"choices":[{"delta":{"content":"metadata-control","annotations":[{"source_index":` + count + `}]},"logprobs":{"token_count":` + count + `},"finish_reason":"stop"}]}`
			wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader("data: "+packet+"\n\ndata: {\"choices\":[{\"delta\":{},\"logprobs\":null}]}\n\ndata: [DONE]\n\n")))
			const prefix = "event: response.completed\r\ndata: "
			index := strings.Index(wire, prefix)
			if index < 0 || strings.Contains(wire, "event: response.failed") {
				t.Fatalf("metadata stream did not complete: %s", wire)
			}
			data := strings.TrimSuffix(wire[index+len(prefix):], "\r\n\r\n")
			var event map[string]any
			decoder := json.NewDecoder(bytes.NewBufferString(data))
			decoder.UseNumber()
			if err := decoder.Decode(&event); err != nil {
				t.Fatal(err)
			}
			response, _ := event["response"].(map[string]any)
			metadata, _ := response["metadata"].(map[string]any)
			provider, _ := metadata["deepseek"].(map[string]any)
			logprobs, _ := provider["logprobs"].(map[string]any)
			annotations, _ := provider["annotations"].([]any)
			if len(annotations) != 1 || logprobs["token_count"] != json.Number(count) {
				t.Fatalf("stream metadata snapshot rounded or lost: %#v", provider)
			}
			annotation, _ := annotations[0].(map[string]any)
			if annotation["source_index"] != json.Number(count) {
				t.Errorf("annotation counter rounded: %#v", annotation)
			}
		})
	}
}
