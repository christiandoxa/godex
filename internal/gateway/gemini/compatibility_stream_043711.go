package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// The tagged Prodex Gemini *API-key chat-compatible* path translates
// Chat Completions SSE rather than interpreting the stream as native Gemini
// GenerateContent. Native Gemini chunks contain no choices and therefore
// cannot materialize output_text delta events.
func translateGeminiChatCompatibleStream043711(response *http.Response) *proxymodel.Response {
	reader, writer := io.Pipe()
	go pumpGeminiChatCompatibleStream043711(response.Body, writer)
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "text/event-stream"),
		Body:       reader,
		Trailer:    response.Trailer,
	}
}

type geminiChatCompatibleStream043711 struct {
	id       string
	created  int64
	sequence int
	done     bool
	text     strings.Builder
}

func (state *geminiChatCompatibleStream043711) send(writer io.Writer, event string, fields map[string]any) error {
	fields["type"] = event
	fields["sequence_number"] = state.sequence
	state.sequence++
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: %s\r\ndata: %s\r\n\r\n", event, body)
	return err
}

func (state *geminiChatCompatibleStream043711) complete(writer io.Writer) error {
	if state.done {
		return nil
	}
	state.done = true
	output := make([]any, 0)
	if state.text.Len() > 0 {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{
				"type": "output_text", "text": state.text.String(),
			}},
		})
	}
	return state.send(writer, "response.completed", map[string]any{
		"created_at": state.created,
		"response":   map[string]any{"id": state.id, "output": output},
	})
}

func (state *geminiChatCompatibleStream043711) failure(writer io.Writer, message string) {
	if state.done {
		return
	}
	state.done = true
	_ = state.send(writer, "response.failed", map[string]any{
		"response": map[string]any{
			"id": state.id,
			"error": map[string]any{
				"code": "provider_stream_error", "message": message,
			},
		},
	})
}

func (state *geminiChatCompatibleStream043711) accept(writer io.Writer, data []byte) error {
	if state.done {
		return nil
	}
	translated, supported, err := chatcompat.TranslateChatSSEData(bytes.TrimSpace(data))
	if err != nil {
		return err
	}
	if !supported {
		return nil
	}
	// TranslateChatSSEData returns a single Responses SSE frame. Decode its
	// bounded payload, attach the canonical sequence, and retain visible text
	// for the final response. Gemini-native chunks are ignored above.
	decoder := sse.NewDecoder(1 << 20)
	messages := decoder.Feed(translated)
	messages = append(messages, decoder.Finish()...)
	for _, raw := range messages {
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		name, _ := event["type"].(string)
		switch name {
		case "response.completed":
			return state.complete(writer)
		case "response.output_text.delta":
			if text, ok := event["delta"].(string); ok {
				state.text.WriteString(text)
			}
			if err := state.send(writer, name, event); err != nil {
				return err
			}
		case "response.function_call_arguments.delta", "response.reasoning_summary_text.delta":
			if err := state.send(writer, name, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func pumpGeminiChatCompatibleStream043711(source io.ReadCloser, writer *io.PipeWriter) {
	defer source.Close()
	defer writer.Close()
	state := &geminiChatCompatibleStream043711{
		id: "resp_gemini_" + uuid.NewString(), created: time.Now().Unix(),
	}
	if err := state.send(writer, "response.created", map[string]any{
		"created_at": state.created,
		"response":   map[string]any{"id": state.id},
	}); err != nil {
		return
	}
	decoder := sse.NewDecoder(1 << 20)
	buffer := make([]byte, 32<<10)
	for {
		n, err := source.Read(buffer)
		if n > 0 {
			for _, payload := range decoder.Feed(buffer[:n]) {
				if failure := state.accept(writer, payload); failure != nil {
					state.failure(writer, "provider stream failed")
					return
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				for _, payload := range decoder.Finish() {
					if failure := state.accept(writer, payload); failure != nil {
						state.failure(writer, "provider stream failed")
						return
					}
				}
				if !state.done {
					state.failure(writer, "unexpected end of stream")
				}
			} else {
				state.failure(writer, "provider stream failed")
			}
			return
		}
	}
}
