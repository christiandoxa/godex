package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	compactInstructions               = "Compact the supplied coding-agent transcript into one durable continuation summary. Preserve the user's goals, repository instructions, decisions, files changed, exact identifiers, commands and test results, unresolved failures, current worktree state, and the next concrete steps. Remove redundant narration and obsolete intermediate reasoning. Do not call tools. Return only the continuation summary, with no preamble or completion claim."
	compactModel                      = "chat-compression-default"
	semanticCompactActiveUserMaxBytes = 2 * 1024
	semanticCompactLatestToolMaxBytes = 1024
	semanticCompactSummaryMaxBytes    = 24 * 1024
)

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
	summary = semanticCompactContinuationSummary(summary, original)
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
	value["model"] = compactModel
	value["stream"] = false
	value["store"] = false
	value["parallel_tool_calls"] = false
	value["godex_gemini_compaction"] = true
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

func semanticCompactContinuationSummary(semantic string, body []byte) string {
	semantic = strings.TrimSpace(semantic)
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return semantic
	}
	input, _ := value["input"].([]any)
	activeUser := -1
	for index, raw := range input {
		item, _ := raw.(map[string]any)
		if item == nil || item["type"] != "message" || item["role"] != "user" {
			continue
		}
		activeUser = index
	}
	latestTool := -1
	if activeUser >= 0 {
		for index := len(input) - 1; index > activeUser; index-- {
			item, _ := input[index].(map[string]any)
			kind, _ := item["type"].(string)
			switch kind {
			case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
				latestTool = index
				index = activeUser + 1
			}
		}
	}

	var activeText, toolText string
	if activeUser >= 0 {
		item, _ := input[activeUser].(map[string]any)
		activeText = geminiCompactTextFromContent(item["content"])
		activeText = truncateGeminiCompactEdges(activeText, semanticCompactActiveUserMaxBytes)
	}
	if latestTool >= 0 {
		item, _ := input[latestTool].(map[string]any)
		if output, ok := item["output"]; ok {
			toolText = geminiCompactTextFromContent(output)
		} else {
			toolText = geminiCompactTextFromContent(item["content"])
		}
		toolText = truncateGeminiCompactEdges(toolText, semanticCompactLatestToolMaxBytes)
	}

	var summary strings.Builder
	if text := strings.TrimSpace(activeText); text != "" {
		summary.WriteString("Active user request that must still be completed:\n")
		summary.WriteString(text)
		summary.WriteString("\n\n")
	}
	if text := strings.TrimSpace(toolText); text != "" {
		summary.WriteString("Latest tool result after the active request:\n")
		summary.WriteString(text)
		summary.WriteString("\n\n")
	}
	summary.WriteString("Semantic continuation summary:\n")
	summary.WriteString(semantic)
	summary.WriteString("\n\nContinue the active user request. Do not merely acknowledge repository, optimizer, or environment instructions.")
	return truncateGeminiCompactTail(summary.String(), semanticCompactSummaryMaxBytes)
}

func geminiCompactTextFromContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := geminiCompactTextFromContent(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "output", "input", "query", "command", "commands"} {
			if text := geminiCompactTextFromContent(typed[key]); strings.TrimSpace(text) != "" {
				return text
			}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if text := geminiCompactTextFromContent(typed[key]); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case json.Number:
		return typed.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", typed), "0"), ".")
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func truncateGeminiCompactEdges(text string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(text) <= maximum {
		return text
	}
	const separator = "\n[... middle truncated ...]\n"
	if maximum <= len(separator) {
		return separator[:maximum]
	}
	retained := maximum - len(separator)
	headBytes := retained / 3
	tailBytes := retained - headBytes
	headEnd := min(headBytes, len(text))
	for headEnd > 0 && headEnd < len(text) && !utf8.RuneStart(text[headEnd]) {
		headEnd--
	}
	tailStart := max(len(text)-tailBytes, 0)
	for tailStart < len(text) && !utf8.RuneStart(text[tailStart]) {
		tailStart++
	}
	return text[:headEnd] + separator + text[tailStart:]
}

func truncateGeminiCompactTail(text string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(text) <= maximum {
		return text
	}
	const suffix = "\n[truncated]"
	if maximum <= len(suffix) {
		return suffix[:maximum]
	}
	end := maximum - len(suffix)
	for end > 0 && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + suffix
}
