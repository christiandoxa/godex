package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	"github.com/christiandoxa/godex/internal/helper/redact"
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
	id           string
	itemID       string
	created      int64
	sequence     int
	started      bool
	done         bool
	itemOpened   bool
	text         strings.Builder
	model        string
	finishReason string
	usage        map[string]any
}

func (state *geminiChatCompatibleStream043711) send(writer io.Writer, event string, fields map[string]any) error {
	fields["type"] = event
	fields["sequence_number"] = state.sequence
	state.sequence++
	body, err := jsonutil.MarshalSerdeValue04384(fields)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: %s\r\ndata: %s\r\n\r\n", event, body)
	return err
}

func (state *geminiChatCompatibleStream043711) openTextItem(writer io.Writer) error {
	if state.itemOpened {
		return nil
	}
	state.itemOpened = true
	state.itemID = "msg_gemini_" + strconv.FormatUint(rand.Uint64(), 10)
	return state.send(writer, "response.output_item.added", map[string]any{
		"response_id": state.id,
		"item": map[string]any{
			"id": state.itemID, "role": "assistant", "type": "message",
			"content": []any{},
		},
	})
}

func (state *geminiChatCompatibleStream043711) complete(writer io.Writer) error {
	if state.done {
		return nil
	}
	output := make([]any, 0)
	if state.itemOpened {
		contents := []any{map[string]any{
			"type": "output_text", "text": state.text.String(),
		}}
		item := map[string]any{
			"id": state.itemID, "role": "assistant", "type": "message",
			"content": contents,
		}
		if err := state.send(writer, "response.output_item.done", map[string]any{
			"response_id": state.id, "item": item,
		}); err != nil {
			return err
		}
		output = append(output, map[string]any{
			"type": "message", "role": "assistant", "content": contents,
		})
	}
	state.done = true
	completed := map[string]any{"id": state.id, "output": output}
	if state.model != "" {
		completed["model"] = state.model
	}
	if state.finishReason != "" {
		completed["metadata"] = map[string]any{
			"gemini": map[string]any{"finish_reason": state.finishReason},
		}
	}
	if state.usage != nil {
		completed["usage"] = state.usage
	}
	return state.send(writer, "response.completed", map[string]any{
		"created_at": state.created,
		"response":   completed,
	})
}

func (state *geminiChatCompatibleStream043711) failure(writer io.Writer, code, message string) error {
	if state.done {
		return nil
	}
	state.done = true
	return state.send(writer, "response.failed", map[string]any{
		"created_at": state.created,
		"response": map[string]any{
			"id": state.id,
			"error": map[string]any{
				"code": code, "message": redact.Secrets(message),
			},
		},
	})
}

func (state *geminiChatCompatibleStream043711) observeChatChunk(chunk map[string]any) {
	if model, ok := chunk["model"].(string); ok && model != "" {
		state.model = model
	}
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if finish, ok := first["finish_reason"].(string); ok && finish != "" {
				state.finishReason = finish
			}
		}
	}
	if provider, ok := chunk["usage"].(map[string]any); ok {
		state.usage = map[string]any{
			"input_tokens":  provider["prompt_tokens"],
			"output_tokens": provider["completion_tokens"],
			"total_tokens":  provider["total_tokens"],
		}
	}
}

func (state *geminiChatCompatibleStream043711) accept(writer io.Writer, data []byte) error {
	if state.done {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return state.complete(writer)
	}
	root, valid := geminiSerdeResponseObject04384(data)
	if !valid {
		return errors.New("failed to parse Gemini chat completions SSE JSON")
	}
	if value, present := root["error"]; present {
		code, message := chatcompat.ChatStreamError(value)
		return state.failure(writer, code, message)
	}
	if !state.started {
		state.started = true
		if err := state.send(writer, "response.created", map[string]any{
			"created_at": state.created, "response": map[string]any{"id": state.id},
		}); err != nil {
			return err
		}
	}
	state.observeChatChunk(root)
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
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&event); err != nil {
			return err
		}
		name, _ := event["type"].(string)
		switch name {
		case "response.completed":
			if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
				return state.complete(writer)
			}
		case "response.output_text.delta":
			if err := state.openTextItem(writer); err != nil {
				return err
			}
			if text, ok := event["delta"].(string); ok {
				state.text.WriteString(text)
			}
			event["created_at"] = state.created
			event["response_id"] = state.id
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
	responseID := "resp_deepseek_" + uuid.NewString()
	if value, err := uuid.NewV7(); err == nil {
		responseID = "resp_deepseek_" + value.String()
	}
	state := &geminiChatCompatibleStream043711{
		id: responseID, created: time.Now().Unix(),
	}
	decoder := sse.NewDecoder(1 << 20)
	buffer := make([]byte, 32<<10)
	for {
		n, err := source.Read(buffer)
		if n > 0 {
			for _, payload := range decoder.Feed(buffer[:n]) {
				if failure := state.accept(writer, payload); failure != nil {
					_ = state.failure(writer, "provider_stream_error", "provider stream failed")
					return
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				for _, payload := range decoder.Finish() {
					if failure := state.accept(writer, payload); failure != nil {
						_ = state.failure(writer, "provider_stream_error", "provider stream failed")
						return
					}
				}
				if !state.done {
					_ = state.failure(writer, "provider_stream_error", "unexpected end of stream")
				}
			} else {
				_ = state.failure(writer, "provider_stream_error", "provider stream failed")
			}
			return
		}
	}
}
