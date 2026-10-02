package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const compactInstructions = "Compact the supplied coding-agent transcript into one durable continuation summary. Preserve the user's goals, repository instructions, decisions, files changed, exact identifiers, commands and test results, unresolved failures, and next concrete steps. Remove redundant narration and obsolete intermediate reasoning. Do not call tools. Return only the continuation summary, with no preamble or completion claim."

func (transport *RuntimeTransport) executeCompact(ctx context.Context, input proxymodel.Request) (*proxymodel.Response, error) {
	original := input.Body
	body, err := semanticCompactRequest(input.Body)
	if err != nil {
		return compactgateway.LocalFallback(original, "gemini", "invalid-request")
	}
	input.Body = body
	response, err := transport.executeResponses(ctx, input, route{kind: routeResponses})
	if err != nil {
		return compactgateway.LocalFallback(original, "gemini", "upstream-error")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response, nil
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil || len(content) > bodyMaxBytes {
		return compactgateway.LocalFallback(original, "gemini", "invalid-response")
	}
	summary := compactSummary(content)
	if summary == "" {
		return compactgateway.LocalFallback(original, "gemini", "no-summary")
	}
	return compactgateway.Semantic(summary, "gemini")
}

func semanticCompactRequest(body []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("invalid Gemini compact request")
	}
	input, ok := value["input"].([]any)
	if !ok {
		return nil, errors.New("Gemini compact request input must be an array")
	}
	input = append(input, map[string]any{"type": "message", "role": "user", "content": []any{
		map[string]any{"type": "input_text", "text": compactInstructions},
	}})
	value["input"] = input
	value["instructions"] = compactInstructions
	model, _ := value["model"].(string)
	if strings.TrimSpace(model) == "" {
		model = "chat-compression-default"
	}
	value["model"] = model
	value["stream"] = false
	value["store"] = false
	value["parallel_tool_calls"] = false
	value["prodex_gemini_compaction"] = true
	for _, key := range []string{"include", "previous_response_id", "prompt_cache_key", "text", "tool_choice", "tools"} {
		delete(value, key)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to serialize Gemini compact request")
	}
	return encoded, nil
}

func compactSummary(body []byte) string {
	var response map[string]any
	if json.Unmarshal(body, &response) != nil {
		return ""
	}
	output, _ := response["output"].([]any)
	var text []string
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] != "message" {
			continue
		}
		content, _ := item["content"].([]any)
		for _, rawPart := range content {
			part, _ := rawPart.(map[string]any)
			if kind, _ := part["type"].(string); kind != "output_text" && kind != "input_text" {
				continue
			}
			if value, ok := part["text"].(string); ok && strings.TrimSpace(value) != "" {
				text = append(text, strings.TrimSpace(value))
			}
		}
	}
	return strings.Join(text, "\n")
}
