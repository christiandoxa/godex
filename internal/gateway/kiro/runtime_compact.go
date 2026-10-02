package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	kiroCompactUnsupported     = "unsupported"
	kiroCompactInvalidResponse = "invalid-response"
)

const semanticCompactInstructions = "Compact the supplied coding-agent transcript into one durable continuation summary. Preserve the user's goals, repository instructions, decisions, files changed, exact identifiers, commands and test results, unresolved failures, current worktree state, and the next concrete steps. Remove redundant narration and obsolete intermediate reasoning. Do not call tools. Return only the continuation summary, with no preamble or completion claim."

func (source *Source) semanticCompact(ctx context.Context, home string, body []byte) (*proxymodel.Response, error) {
	rewritten, err := kiroSemanticCompactRequest(body)
	if err != nil {
		return compactgateway.LocalFallback(body, "kiro", kiroCompactInvalidResponse)
	}
	request, err := parseKiroResponsesRequest(rewritten, true)
	if err != nil {
		return compactgateway.LocalFallback(body, "kiro", kiroCompactUnsupported)
	}
	turn, err := source.executeACPTurn(ctx, home, request.model, request.effort, request.prompt)
	if err != nil {
		return compactgateway.LocalFallback(body, "kiro", kiroCompactReason(err))
	}
	response := kiroResponseFromTurn(turn, 0, request.model, "")
	summary := strings.TrimSpace(kiroResponseText(response))
	if summary == "" {
		return compactgateway.LocalFallback(body, "kiro", kiroCompactInvalidResponse)
	}
	return compactgateway.Semantic(summary, "kiro")
}

func kiroSemanticCompactRequest(body []byte) ([]byte, error) {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return nil, errors.New("failed to parse Kiro compact request JSON")
	}
	input, ok := value["input"].([]any)
	if !ok {
		return nil, errors.New("Kiro compact request must contain an input array")
	}
	input = append(input, map[string]any{
		"type": "message", "role": "user",
		"content": []any{map[string]any{"type": "input_text", "text": semanticCompactInstructions}},
	})
	value["input"] = input
	value["stream"] = false
	value["store"] = false
	for _, key := range []string{"include", "previous_response_id", "prompt_cache_key", "text", "tool_choice", "tools"} {
		delete(value, key)
	}
	return json.Marshal(value)
}

func kiroCompactReason(err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "timeout"):
		return "timeout"
	case strings.Contains(text, kiroCompactUnsupported):
		return kiroCompactUnsupported
	case strings.Contains(text, "parse") || strings.Contains(text, "invalid"):
		return kiroCompactInvalidResponse
	case strings.Contains(text, "unavailable") || strings.Contains(text, "not found"):
		return "unavailable"
	default:
		return "provider-error"
	}
}
