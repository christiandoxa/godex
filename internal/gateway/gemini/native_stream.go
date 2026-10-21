package gemini

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	"github.com/christiandoxa/godex/internal/helper/redact"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

type geminiStreamState struct {
	responseID string
	createdAt  int64
	model      string
	text       strings.Builder
	reasoning  strings.Builder
	toolCalls  []geminiStreamToolCall
	toolBytes  int
	usage      map[string]any
	metadata   map[string]any
	finish     string
	sequence   uint64
	completed  bool
}

func translateGeminiNativeStream(response *http.Response, requestMetadata map[string]any) *proxymodel.Response {
	reader, writer := io.Pipe()
	go pumpGeminiNativeStream(response.Body, writer, requestMetadata)
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "text/event-stream"),
		Body:       reader,
		Trailer:    response.Trailer,
	}
}

func pumpGeminiNativeStream(body io.ReadCloser, writer *io.PipeWriter, requestMetadata map[string]any) {
	defer body.Close()
	state := &geminiStreamState{responseID: "resp_gemini_" + uuid.NewString(), createdAt: time.Now().Unix()}
	decoder := sse.NewDecoder(1 << 20)
	buffer := make([]byte, 32<<10)
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			for _, data := range decoder.Feed(buffer[:n]) {
				if writeErr := state.consume(writer, data, requestMetadata); writeErr != nil {
					_ = state.failed(writer, "provider_stream_error", "Gemini stream failed")
					_ = writer.Close()
					return
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				for _, data := range decoder.Finish() {
					if writeErr := state.consume(writer, data, requestMetadata); writeErr != nil {
						_ = state.failed(writer, "provider_stream_error", "Gemini stream failed")
						_ = writer.Close()
						return
					}
				}
			}
			if !state.completed {
				message := "Gemini stream failed"
				if err == io.EOF {
					message = "unexpected end of Gemini stream"
				}
				_ = state.failed(writer, "provider_stream_error", message)
			}
			_ = writer.Close()
			return
		}
	}
}

func (state *geminiStreamState) failed(writer io.Writer, code, message string) error {
	if state.completed {
		return nil
	}
	state.completed = true
	return writeGeminiSSEEvent(writer, "response.failed", map[string]any{
		"type": "response.failed", "sequence_number": state.nextSequence(),
		"created_at": state.createdAt,
		"response": map[string]any{
			"id":    state.responseID,
			"error": map[string]any{"code": code, "message": redact.Secrets(message)},
		},
	})
}

func (state *geminiStreamState) consume(writer io.Writer, data []byte, requestMetadata map[string]any) error {
	if state.completed {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return state.complete(writer, requestMetadata)
	}
	root, valid := geminiSerdeResponseObject04384(data)
	if !valid {
		return fmt.Errorf("failed to parse Gemini SSE JSON")
	}
	if code, message, failed := geminiNativeStreamError(root); failed {
		return state.failed(writer, code, message)
	}
	state.applyChunkMetadata(root)
	if message, blocked := geminiNativePromptFailure(root); blocked {
		return state.failed(writer, "gemini_prompt_blocked", message)
	}
	candidate := firstNativeCandidate(root)
	parts := nativeCandidateParts(candidate)
	if parts != nil {
		for index, raw := range parts {
			part, _ := raw.(map[string]any)
			if part == nil {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				if thought, _ := part["thought"].(bool); thought {
					state.reasoning.WriteString(text)
					if err := writeGeminiSSEEvent(writer, "response.reasoning_summary_text.delta", map[string]any{
						"type": "response.reasoning_summary_text.delta", "delta": text,
						"sequence_number": state.nextSequence(),
					}); err != nil {
						return err
					}
				} else {
					state.text.WriteString(text)
					if err := writeGeminiSSEEvent(writer, "response.output_text.delta", map[string]any{
						"type": "response.output_text.delta", "delta": text,
						"sequence_number": state.nextSequence(),
					}); err != nil {
						return err
					}
				}
			}
			if call, ok := part["functionCall"].(map[string]any); ok {
				if err := state.observeTool(writer, index, part, call); err != nil {
					return err
				}
			}
		}
	}
	if parts != nil && nativeNonblank(candidate["finishReason"]) != "" {
		return state.finishChunk(writer, root, candidate)
	}
	return nil
}

func (state *geminiStreamState) applyChunkMetadata(root map[string]any) {
	if strings.HasPrefix(state.responseID, "resp_gemini_") {
		if value, exists := root["responseId"]; exists {
			if responseID, ok := value.(string); ok {
				state.responseID = responseID
			}
		} else if value, exists := root["id"]; exists {
			if responseID, ok := value.(string); ok {
				state.responseID = responseID
			}
		}
	}
	if value, exists := root["modelVersion"]; exists {
		if model, ok := value.(string); ok {
			state.model = model
		}
	} else if value, exists := root["model"]; exists {
		if model, ok := value.(string); ok {
			state.model = model
		}
	}
	if value, exists := root["usageMetadata"]; exists {
		usage, _ := value.(map[string]any)
		state.usage = geminiNativeUsage(usage)
	}
	candidate := firstNativeCandidate(root)
	if provider := geminiNativeResponseMetadata(root, candidate); len(provider) > 0 {
		if state.metadata == nil {
			state.metadata = make(map[string]any)
		}
		// Mojo merges the outer gemini object, replacing each provider field.
		for key, value := range provider {
			state.metadata[key] = value
		}
	}
	if candidate != nil {
		if finish, ok := candidate["finishReason"].(string); ok && strings.TrimSpace(finish) != "" {
			state.finish = finish
		}
	}
}

func (state *geminiStreamState) nextSequence() uint64 {
	value := state.sequence
	state.sequence++
	return value
}

func (state *geminiStreamState) complete(writer io.Writer, requestMetadata map[string]any) error {
	if state.completed {
		return nil
	}
	if err := state.completeTools(writer); err != nil {
		return err
	}
	state.completed = true
	output := make([]any, 0)
	if state.text.Len() > 0 {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": state.text.String()}},
		})
	}
	for _, tool := range state.toolCalls {
		output = append(output, tool.item)
	}
	response := map[string]any{
		"id": state.responseID, "object": "response", "output": output,
	}
	if state.model != "" {
		response["model"] = state.model
	}
	if state.usage != nil {
		response["usage"] = state.usage
	}
	metadata := make(map[string]any)
	mergeMetadataFields(metadata, requestMetadata)
	if len(state.metadata) > 0 {
		metadata["gemini"] = state.metadata
	}
	if len(metadata) > 0 {
		response["metadata"] = metadata
	}
	if state.finish == "MAX_TOKENS" {
		response["status"] = "incomplete"
		response["incomplete_details"] = geminiMaxTokensIncompleteDetails()
	}
	return writeGeminiSSEEvent(writer, "response.completed", map[string]any{
		"type": "response.completed", "response": response,
		"sequence_number": state.nextSequence(),
	})
}

func writeGeminiSSEEvent(writer io.Writer, event string, payload map[string]any) error {
	// Prodex emits the translated event through serde_json's canonical
	// serializer. Do not introduce Go-only HTML or JavaScript escapes.
	encoded, err := jsonutil.MarshalSerdeValue04384(payload)
	if err != nil {
		return err
	}
	buffer := bufio.NewWriter(writer)
	if _, err := fmt.Fprintf(buffer, "event: %s\r\ndata: %s\r\n\r\n", event, encoded); err != nil {
		return err
	}
	return buffer.Flush()
}
