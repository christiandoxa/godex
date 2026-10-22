package gemini

import (
	"context"
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
func translateGeminiChatCompatibleStream043711(ctx context.Context, response *http.Response, requestMetadata map[string]any, conversation *geminiChatConversation04384) *proxymodel.Response {
	ctx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	body := sse.NewPipeBody(reader, &geminiChatStreamSource04384{source: response.Body, cancel: cancel})
	go func() {
		defer body.CloseSource()
		pumpGeminiChatCompatibleStream043711(ctx, response.Body, writer, requestMetadata, conversation)
	}()
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "text/event-stream; charset=utf-8"),
		Body:       body,
		Trailer:    response.Trailer,
	}
}

// Bound retained completion metadata to the conversation cache's memory budget.
// shortcut: retained metadata is capped at 64 MiB per collection; qualify larger streams before raising it.
const geminiStreamMetadataMaxBytes04384 = 64 << 20

type geminiChatCompatibleStream043711 struct {
	id           string
	itemID       string
	created      uint64
	sequence     int
	started      bool
	done         bool
	eof          bool
	itemOpened   bool
	text         strings.Builder
	reasoning    strings.Builder
	model        string
	modelPresent bool
	finishReason string
	finishSet    bool
	fingerprint  string
	logprobs     any
	usage        map[string]any
	metadata     map[string]any
	json         sse.JSONFrames
	conversation *geminiChatConversation04384

	refusal              strings.Builder
	annotations          []any
	messageMetadataBytes int
	toolCalls            map[uint64]*geminiChatStreamTool04384
	toolBytes            int
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
	if valid, err := state.completeToolEvents(writer); err != nil || !valid {
		return err
	}
	if err := state.completeReasoning(writer); err != nil {
		return err
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
	for _, index := range state.orderedToolIndices() {
		output = append(output, geminiChatCompletedToolItem04384(state.toolCalls[index]))
	}
	state.done = true
	completed := map[string]any{"id": state.id, "output": output}
	if state.modelPresent {
		completed["model"] = state.model
	}
	provider := make(map[string]any)
	if state.finishSet {
		provider["finish_reason"] = state.finishReason
	}
	if state.reasoning.Len() > 0 {
		provider["reasoning_content"] = state.reasoning.String()
	}
	if state.refusal.Len() > 0 {
		provider["refusal"] = state.refusal.String()
	}
	if len(state.annotations) > 0 {
		provider["annotations"] = state.annotations
	}
	if state.fingerprint != "" {
		provider["system_fingerprint"] = state.fingerprint
	}
	if state.logprobs != nil {
		provider["logprobs"] = state.logprobs
	}
	if len(provider) > 0 {
		completed["metadata"] = map[string]any{"gemini": provider}
	}
	if len(state.metadata) != 0 {
		metadata, _ := completed["metadata"].(map[string]any)
		if metadata == nil {
			metadata = make(map[string]any, len(state.metadata))
		}
		mergeGeminiChatMetadata04384(metadata, state.metadata)
		completed["metadata"] = metadata
	}
	if state.usage != nil {
		completed["usage"] = state.usage
	}
	state.rememberConversation()
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
	if created, ok := nativeUintPresent(chunk["created"]); ok {
		state.created = created
	}
	if fingerprint, _ := chunk["system_fingerprint"].(string); fingerprint != "" {
		state.fingerprint = fingerprint
	}
	if model, ok := chunk["model"].(string); ok {
		state.model, state.modelPresent = model, true
	}
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if logprobs := first["logprobs"]; logprobs != nil {
				state.logprobs = logprobs
			}
			if finish, ok := first["finish_reason"].(string); ok {
				state.finishReason, state.finishSet = finish, true
			}
		}
	}
	if usage, present := chunk["usage"]; present {
		state.usage = chatcompat.ResponsesUsage(usage, "gemini")
	}
}

func (state *geminiChatCompatibleStream043711) accept(writer io.Writer, data []byte) error {
	if state.eof {
		return nil
	}
	if sse.JSONDone(data) {
		state.eof = true
		if state.json.Pending() {
			return state.failure(writer, "provider_stream_error", "incomplete JSON segment")
		}
		return state.complete(writer)
	}
	root, valid := geminiSerdeResponseObject04384(data)
	if !valid {
		values, err := state.json.Observe(data)
		if err != nil {
			state.eof = true
			return state.failure(writer, "provider_stream_error", err.Error())
		}
		for _, value := range values {
			if err := state.accept(writer, value); err != nil {
				return err
			}
		}
		return nil
	}
	if value, present := root["error"]; present {
		code, message := chatcompat.ChatStreamError(value)
		return state.failure(writer, code, message)
	}
	if pendingGeminiChatProjection04384(root) {
		return geminiChatProjectionPending04384{}
	}
	state.observeChatChunk(root)
	if !state.started {
		state.started = true
		if err := state.send(writer, "response.created", map[string]any{
			"created_at": state.created, "response": map[string]any{"id": state.id},
		}); err != nil {
			return err
		}
	}
	if err := state.observeReasoning(writer, root); err != nil || state.eof {
		return err
	}
	if err := state.observeMessageMetadata(writer, root); err != nil || state.eof {
		return err
	}
	return state.observeChoiceOutput(writer, root)
}

func pumpGeminiChatCompatibleStream043711(ctx context.Context, source io.ReadCloser, writer *io.PipeWriter, requestMetadata map[string]any, conversation *geminiChatConversation04384) {
	defer writer.Close()
	responseID := "resp_deepseek_" + uuid.NewString()
	if value, err := uuid.NewV7(); err == nil {
		responseID = "resp_deepseek_" + value.String()
	}
	state := &geminiChatCompatibleStream043711{
		id: responseID, created: uint64(max(0, time.Now().Unix())), metadata: requestMetadata, conversation: conversation,
	}
	decoder := sse.NewStrictDecoder(4 << 20)
	buffer := make([]byte, 32<<10)
	for {
		n, err := source.Read(buffer)
		if n > 0 {
			for _, payload := range decoder.Feed(buffer[:n]) {
				if failure := state.accept(writer, payload); failure != nil {
					state.handleTranslationFailure(ctx, writer, failure)
					return
				}
			}
			if state.eof {
				return
			}
		}
		if decoder.Err() != nil {
			_ = state.failure(writer, "provider_stream_error", decoder.Err().Error())
			return
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				for _, payload := range decoder.Finish() {
					if failure := state.accept(writer, payload); failure != nil {
						state.handleTranslationFailure(ctx, writer, failure)
						return
					}
				}
				if !state.done {
					message := "unexpected end of stream"
					if decoder.Err() != nil {
						message = decoder.Err().Error()
					} else if state.json.Pending() {
						message = "incomplete JSON segment"
					}
					_ = state.failure(writer, "provider_stream_error", message)
				}
			} else {
				_ = state.failure(writer, "provider_stream_error", "provider stream failed")
			}
			return
		}
	}
}
