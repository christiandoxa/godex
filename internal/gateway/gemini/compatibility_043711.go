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
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const bufferedChatResponseMaxBytes = 4 << 20

// The tagged Prodex 0.437.11 API-key Super path is the Gemini
// OpenAI-compatible Chat Completions bridge. OAuth/native Gemini routes use
// the separate generateContent adapter and are left untouched here.
func (transport *RuntimeTransport) executeChatCompatibleResponses(
	ctx context.Context, input proxymodel.Request, current route,
) (*proxymodel.Response, error) {
	normalized, controls, err := normalizeGeminiChatInput043711(input.Body)
	if err != nil {
		var failure *proxymodel.Error
		if errors.As(err, &failure) {
			// The released API-key bridge returns an empty 500 for valid nonobject roots.
			if failure.StatusCode == http.StatusInternalServerError {
				return &proxymodel.Response{StatusCode: http.StatusInternalServerError,
					Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil)),
					PrecommitFailure: &proxymodel.PrecommitFailure{Code: "invalid_request_error"}}, nil
			}
			return nil, failure
		}
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	metadata, err := geminiChatResponseMetadata04384(input.Body, controls)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	body, err := chatcompat.ResponsesRequestPreservingMessages(normalized, requestModel(input.Body), "")
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	body, err = filterGeminiOrphanToolMessages043711(body)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	body, err = geminiChatRequestOptions04384(body, normalized, controls)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	response, err := transport.send(ctx, input, current, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return proxyResponse(response), nil
	}
	if geminiContentTypeIsSSE04384(response.Header.Get("Content-Type")) {
		return translateGeminiChatCompatibleStream043711(response, metadata), nil
	}
	translated, err := translateGeminiChatOrNativeResponse043711(response, metadata)
	if err != nil {
		return &proxymodel.Response{
			StatusCode:       http.StatusBadGateway,
			Header:           http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
			Body:             io.NopCloser(bytes.NewBufferString("provider response could not be processed")),
			PrecommitFailure: &proxymodel.PrecommitFailure{Code: "provider_response_translation_failed"},
		}, nil
	}
	return translated, nil
}

// Tagged Gemini uses user_id on the Chat wire and requests final token
// usage for streaming requests.
func geminiChatRequestOptions04384(body, input []byte, controls map[string]any) ([]byte, error) {
	request, err := decodeGeminiChatRequest04384(body)
	if err != nil {
		return nil, errors.New("invalid Gemini chat-compatible stream request")
	}
	if err := restoreGeminiChatCallSignatures04384(request, input); err != nil {
		return nil, err
	}
	if geminiChatThinking04384(controls) {
		normalizeGeminiChatThinkingMessages04384(request)
	}
	if controls["response_format"] != nil {
		ensureGeminiChatJSONInstruction04384(request)
	}
	user, hasUser := request["user"]
	if hasUser {
		request["user_id"] = user
		delete(request, "user")
	}
	for key, value := range controls {
		request[key] = value
	}
	stream, _ := request["stream"].(bool)
	if stream {
		options, _ := request["stream_options"].(map[string]any)
		if options == nil {
			options = make(map[string]any)
		}
		options["include_usage"] = true
		request["stream_options"] = options
	}
	return jsonutil.MarshalSerdeValue04384(request)
}

// Responses input entries may specify only a role and content, with no
// explicit "type":"message". The tagged provider treats them as messages.
func normalizeGeminiChatInput043711(body []byte) ([]byte, map[string]any, error) {
	// A malformed complete JSON value cannot become valid merely because
	// Go normalized/re-serialized an unrelated ignored field into U+FFFD
	// or a differently rounded f64 before calling the provider.
	if !geminiSerdeJSONValid04384(body) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	root, err := decodeGeminiChatRequest04384(body)
	var wrongRoot *json.UnmarshalTypeError
	if err == nil && root == nil || errors.As(err, &wrongRoot) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusInternalServerError}
	}
	if err != nil {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	if _, _, err := geminiCandidateCount(root); err != nil {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	if err := validateGeminiChatCompatibleTools04380(body); err != nil {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	if !validGeminiChatControlTypes04384(root) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	if !normalizeGeminiTokenLimit04384(root) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	controls, valid := extractGeminiChatLogprobs04384(root)
	if !valid {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	controls, valid = extractGeminiChatStop04384(root, controls)
	if !valid {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	controls, valid = normalizeGeminiChatReasoning04384(root, controls)
	if !valid {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	controls, valid = normalizeGeminiChatFormat04384(root, controls)
	if !valid {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	if !normalizeGeminiChatFunctionDeclarations04384(root) || !normalizeGeminiChatChoice04384(root) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	if err := normalizeGeminiUserID04384(root); err != nil {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	// Routing rejects bound raw-key IDs; unbound IDs use the tagged fresh-input fallback.
	delete(root, "previous_response_id")
	delete(root, "messages")
	for _, field := range []string{"parallel_tool_calls", "presence_penalty", "frequency_penalty", "seed", "n"} {
		delete(root, field)
	}
	// The original response-only fields are validated before shared Chat conversion.
	for _, field := range []string{"metadata", "client_metadata", "prompt_cache_key", "prompt_cache_retention"} {
		delete(root, field)
	}
	if instructions, ok := root["instructions"].(string); ok && strings.TrimSpace(instructions) == "" {
		delete(root, "instructions")
	}
	if !normalizeGeminiChatInputItems04384(root) {
		return nil, nil, &proxymodel.Error{StatusCode: http.StatusBadGateway, Message: "upstream request failed"}
	}
	_, hasInput := root["input"]
	entries, isArray := root["input"].([]any)
	if _, hasInstructions := root["instructions"]; !hasInstructions && (!hasInput || isArray && len(entries) == 0) {
		root["input"] = ""
	}
	normalized, err := json.Marshal(root)
	return normalized, controls, err
}

func decodeGeminiChatRequest04384(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("Gemini chat request must contain one JSON value")
	}
	return request, nil
}

func validGeminiChatControlTypes04384(request map[string]any) bool {
	for _, field := range []string{"prefix", "suffix", "prompt"} {
		if _, present := request[field]; present {
			return false
		}
	}
	for _, field := range []string{"temperature", "top_p"} {
		if value, present := request[field]; present {
			if _, valid := value.(json.Number); !valid {
				return false
			}
		}
	}
	if value, present := request["stream"]; present {
		if _, valid := value.(bool); !valid {
			return false
		}
	}
	for _, field := range []string{"instructions", "previous_response_id"} {
		if value, present := request[field]; present {
			if _, valid := value.(string); !valid {
				return false
			}
		}
	}
	return true
}

// Tagged Prodex 0.437.11 uses the Gemini *native* response normalizer
// even for API-key requests sent to the OpenAI-compatible chat endpoint.
// A Chat Completion-shaped JSON response has no Gemini candidates. The
// official provider-transform fallback does NOT reuse the chat completion's
// ID, model, or usage: it emits an empty Gemini error envelope.
func translateGeminiChatOrNativeResponse043711(
	response *http.Response, requestMetadata map[string]any,
) (*proxymodel.Response, error) {
	if geminiContentTypeIsSSE04384(response.Header.Get("Content-Type")) {
		return translateResponse(response, requestMetadata)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, bufferedChatResponseMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Gemini chat-compatible response")
	}
	if len(payload) > bufferedChatResponseMaxBytes {
		return nil, errors.New("Gemini chat-compatible response exceeded its read limit")
	}
	root, valid := geminiSerdeResponseObject04384(payload)
	if !valid {
		return nil, errors.New("failed to parse Gemini chat-compatible response JSON")
	}
	_, hasEnvelope := root["response"]
	if _, hasCandidates := root["candidates"]; !hasCandidates && !hasEnvelope {
		if _, hasChoices := root["choices"]; hasChoices {
			// This is the exact tagged provider translator's fallback for
			// an upstream Chat Completion that cannot be decoded as native
			// Gemini GenerateContent. Never fabricate successful text.
			return geminiChatFallbackResponse04384(response.StatusCode, requestMetadata)
		}
	}
	native := *response
	native.Body = io.NopCloser(bytes.NewReader(payload))
	translated, err := translateResponseWithCallFallback04380(&native, requestMetadata, true)
	if err != nil {
		return nil, err
	}
	return normalizeGeminiAPIKeyBufferedIdentity043711(translated, root)
}
