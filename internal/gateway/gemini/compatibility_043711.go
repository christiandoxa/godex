package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
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
	if err := validateGeminiChatCompatibleTools04380(body); err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	body, err = filterGeminiOrphanToolMessages043711(body)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	body, err = geminiChatStreamOptions043711(body)
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
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return translateGeminiChatCompatibleStream043711(response), nil
	}
	return translateGeminiChatOrNativeResponse043711(response)
}

// The tagged 0.437.11 Super Gemini chat-compatible provider asks the
// upstream to return final token usage in all streaming requests.
func geminiChatStreamOptions043711(body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, errors.New("invalid Gemini chat-compatible stream request")
	}
	if stream, _ := request["stream"].(bool); !stream {
		return body, nil
	}
	options, _ := request["stream_options"].(map[string]any)
	if options == nil {
		options = make(map[string]any)
	}
	options["include_usage"] = true
	request["stream_options"] = options
	return json.Marshal(request)
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

// Tagged Prodex 0.437.11 uses the Gemini *native* response normalizer
// even for API-key requests sent to the OpenAI-compatible chat endpoint.
// A Chat Completion-shaped JSON response has no Gemini candidates. The
// official provider-transform fallback does NOT reuse the chat completion's
// ID, model, or usage: it emits an empty Gemini error envelope.
func translateGeminiChatOrNativeResponse043711(
	response *http.Response,
) (*proxymodel.Response, error) {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return translateResponse(response, nil)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Gemini chat-compatible response")
	}
	if len(payload) > bodyMaxBytes {
		return nil, errors.New("Gemini chat-compatible response exceeded its read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return nil, errors.New("failed to parse Gemini chat-compatible response JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("failed to parse Gemini chat-compatible response JSON")
	}
	if _, hasCandidates := root["candidates"]; !hasCandidates {
		if _, hasChoices := root["choices"]; hasChoices {
			// This is the exact tagged provider translator's fallback for
			// an upstream Chat Completion that cannot be decoded as native
			// Gemini GenerateContent. Never fabricate successful text.
			return jsonResponse(response.StatusCode, map[string]any{
				"id":       "gemini_resp_prodex",
				"model":    "gemini-2.5-pro",
				"object":   "response",
				"output":   []any{},
				"metadata": map[string]any{},
				"usage":    map[string]any{},
				"status":   "failed",
				"error": map[string]any{
					"code":    "gemini_empty_response",
					"message": "Gemini returned no visible response content.",
				},
			})
		}
	}
	native := *response
	native.Body = io.NopCloser(bytes.NewReader(payload))
	translated, err := translateResponse(&native, nil)
	if err != nil {
		return nil, err
	}
	return normalizeGeminiAPIKeyBufferedIdentity043711(translated)
}
