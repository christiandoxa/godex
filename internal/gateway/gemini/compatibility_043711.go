package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// The tagged Prodex 0.437.11 API-key Super path is the Gemini
// OpenAI-compatible Chat Completions bridge. OAuth/native Gemini routes use
// the separate generateContent adapter and are left untouched here.
func (transport *RuntimeTransport) executeChatCompatibleResponses(
	ctx context.Context, input proxymodel.Request, current route,
) (*proxymodel.Response, error) {
	normalized, err := normalizeGeminiChatInput043711(input.Body)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	body, err := chatcompat.ResponsesRequest(normalized, requestModel(input.Body), "")
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	body, err = filterGeminiOrphanToolMessages043711(body)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	response, err := transport.send(ctx, input, current, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return proxyResponse(response), nil
	}
	return translateGeminiChatOrNativeResponse043711(response)
}

// Responses input entries may specify only a role and content, with no
// explicit "type":"message". The tagged provider treats them as messages.
func normalizeGeminiChatInput043711(body []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, errors.New("invalid Gemini Responses request")
	}
	if entries, ok := root["input"].([]any); ok {
		for _, raw := range entries {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, hasKind := item["type"]; hasKind {
				continue
			}
			if role, _ := item["role"].(string); role != "" {
				item["type"] = "message"
			}
		}
	}
	return json.Marshal(root)
}

// The tagged chat-compatible adapter repairs tool-call adjacency before
// sending Gemini's OpenAI wire body. Standalone assistant tool calls lacking
// any corresponding tool output cannot be replayed as continuation context.
// Keep complete call/output pairs and ordinary assistant prose.
func filterGeminiOrphanToolMessages043711(body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, errors.New("invalid Gemini chat-compatible request")
	}
	messages, _ := request["messages"].([]any)
	outputIDs := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "tool" {
			if callID, _ := message["tool_call_id"].(string); callID != "" {
				outputIDs[callID] = true
			}
		}
	}
	filtered := make([]any, 0, len(messages))
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "assistant" {
			filtered = append(filtered, raw)
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		if len(calls) == 0 {
			filtered = append(filtered, raw)
			continue
		}
		kept := make([]any, 0, len(calls))
		for _, candidate := range calls {
			call, _ := candidate.(map[string]any)
			id, _ := call["id"].(string)
			if outputIDs[id] {
				kept = append(kept, candidate)
			}
		}
		if len(kept) == 0 {
			delete(message, "tool_calls")
			if text, _ := message["content"].(string); strings.TrimSpace(text) == "" {
				continue
			}
		} else {
			message["tool_calls"] = kept
		}
		filtered = append(filtered, message)
	}
	request["messages"] = filtered
	return json.Marshal(request)
}

func translateGeminiChatOrNativeResponse043711(
	response *http.Response,
) (*proxymodel.Response, error) {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return &proxymodel.Response{
			StatusCode: response.StatusCode,
			Header:     translatedHeaders(response.Header, "text/event-stream"),
			Body:       chatcompat.ChatSSEWithMetadata(response.Body, "gemini", nil),
			Trailer:    response.Trailer,
		}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Gemini chat-compatible response")
	}
	if len(body) > bodyMaxBytes {
		return nil, errors.New("Gemini chat-compatible response exceeded its read limit")
	}
	var shape struct {
		Candidates json.RawMessage `json:"candidates"`
		Choices    json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(body, &shape); err != nil {
		return nil, errors.New("failed to parse Gemini chat-compatible response")
	}
	if len(shape.Candidates) > 0 {
		// The native response adapter is also used by the reference for
		// accepted generateContent envelopes on its chat-compatible URL.
		native := *response
		native.Body = io.NopCloser(bytes.NewReader(body))
		return translateResponse(&native, nil)
	}
	if len(shape.Choices) == 0 {
		return nil, errors.New("Gemini chat-compatible response has no choices or candidates")
	}
	translated, err := chatcompat.ChatResponseWithOptions(body, time.Now(),
		chatcompat.ResponseOptions{
			ProviderKey:        "gemini",
			AdapterLabel:       "Gemini",
			DefaultModel:       "gemini-3.1-pro-preview",
			FallbackResponseID: func() string { return "resp_gemini_" + uuid.NewString() },
		})
	if err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "application/json"),
		Body:       io.NopCloser(bytes.NewReader(translated)),
		Trailer:    response.Trailer,
	}, nil
}
