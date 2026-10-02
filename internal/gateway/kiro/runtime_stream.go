package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func kiroStreamResponse(route runtimeRoute, response map[string]any, requestID uint64, requestedModel string) (*proxymodel.Response, error) {
	var content []byte
	switch route.kind {
	case routeResponses:
		content = kiroResponsesSSE(response)
	case routeChat:
		content = kiroChatSSE(kiroChatResponse(response, requestID))
	case routeMessages:
		content = kiroMessagesSSE(kiroMessagesResponse(response, requestedModel))
	default:
		return nil, fmt.Errorf("Kiro route %d does not support streaming", route.kind)
	}
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(content)), Trailer: make(http.Header)}, nil
}

func kiroResponsesSSE(response map[string]any) []byte {
	var output bytes.Buffer
	if text := kiroResponseText(response); text != "" {
		writeSSE(&output, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", kiroFieldDelta: text})
	}
	writeSSE(&output, "response.completed", map[string]any{"type": "response.completed", "response": response})
	return output.Bytes()
}

func kiroChatSSE(response map[string]any) []byte {
	id := responseString(response["id"], "chatcmpl_kiro_0")
	model := responseString(response[kiroFieldModel], "kiro-cli")
	created := responseUint(response[kiroFieldCreated])
	choices, _ := response[kiroFieldChoices].([]any)
	var message map[string]any
	finish := "stop"
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		message, _ = choice[kiroFieldMessage].(map[string]any)
		finish = responseString(choice[kiroFieldFinishReason], "stop")
	}
	text := ""
	if message != nil {
		text, _ = message[kiroFieldContent].(string)
	}
	var output bytes.Buffer
	if text != "" {
		writeSSEData(&output, map[string]any{
			"id": id, "object": "chat.completion.chunk", kiroFieldCreated: created, kiroFieldModel: model,
			kiroFieldChoices: []any{map[string]any{kiroFieldIndex: 0, kiroFieldDelta: map[string]any{"role": "assistant", kiroFieldContent: text}, kiroFieldFinishReason: nil}},
		})
	}
	writeSSEData(&output, map[string]any{
		"id": id, "object": "chat.completion.chunk", kiroFieldCreated: created, kiroFieldModel: model,
		kiroFieldChoices: []any{map[string]any{kiroFieldIndex: 0, kiroFieldDelta: map[string]any{}, kiroFieldFinishReason: finish}},
	})
	output.WriteString("data: [DONE]\n\n")
	return output.Bytes()
}

func kiroMessagesSSE(message map[string]any) []byte {
	var output bytes.Buffer
	writeSSE(&output, "message_start", map[string]any{"type": "message_start", kiroFieldMessage: map[string]any{
		"id": message["id"], "type": kiroFieldMessage, "role": "assistant", kiroFieldModel: message[kiroFieldModel], kiroFieldContent: []any{}, kiroFieldStopReason: nil, "stop_sequence": nil,
	}})
	content, _ := message[kiroFieldContent].([]any)
	index := 0
	for _, raw := range content {
		part, ok := raw.(map[string]any)
		if !ok || responseString(part["type"], "") != "text" {
			continue
		}
		text := responseString(part["text"], "")
		writeSSE(&output, "content_block_start", map[string]any{"type": "content_block_start", kiroFieldIndex: index, "content_block": map[string]any{"type": "text", "text": ""}})
		if text != "" {
			writeSSE(&output, "content_block_delta", map[string]any{"type": "content_block_delta", kiroFieldIndex: index, kiroFieldDelta: map[string]any{"type": "text_delta", "text": text}})
		}
		writeSSE(&output, "content_block_stop", map[string]any{"type": "content_block_stop", kiroFieldIndex: index})
		index++
	}
	writeSSE(&output, "message_delta", map[string]any{"type": "message_delta", kiroFieldDelta: map[string]any{kiroFieldStopReason: message[kiroFieldStopReason], "stop_sequence": nil}, "usage": message["usage"]})
	writeSSE(&output, "message_stop", map[string]any{"type": "message_stop"})
	return output.Bytes()
}

func writeSSE(output *bytes.Buffer, event string, value any) {
	content, _ := json.Marshal(value)
	fmt.Fprintf(output, "event: %s\ndata: %s\n\n", event, content)
}

func writeSSEData(output *bytes.Buffer, value any) {
	content, _ := json.Marshal(value)
	fmt.Fprintf(output, "data: %s\n\n", content)
}

func isEventStream(header http.Header) bool {
	return strings.Contains(strings.ToLower(header.Get("Content-Type")), "text/event-stream")
}
