package gemini

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func geminiNativeResponsesValue(root map[string]any, requestMetadata map[string]any, createdAt int64) map[string]any {
	return geminiNativeResponsesValueWithFallback04380(root, requestMetadata, createdAt, false)
}

// The public API-key Chat-compatible path has a deterministic fallback
// `call_1` for absent function-call IDs. The direct Gemini GenerateContent
// route keeps its separate synthetic ID behavior; both preserve visible
// text and `functionCall` key presence, including a null value.
func geminiNativeResponsesValueWithFallback04380(
	root map[string]any, requestMetadata map[string]any, createdAt int64, chatCompatible bool,
) map[string]any {
	responseID, model := geminiResponseIdentity04384(root, chatCompatible)
	candidate := firstNativeCandidate(root)
	parts := nativeCandidateParts(candidate)
	output := make([]any, 0)
	messageContent := make([]any, 0)
	var visible strings.Builder
	toolItems := make([]any, 0)
	imageItems := make([]any, 0)
	for index, raw := range parts {
		part, _ := raw.(map[string]any)
		if part == nil {
			continue
		}
		if thought, _ := part["thought"].(bool); !thought {
			if text, ok := part["text"].(string); ok && text != "" && !strings.HasPrefix(text, "data:") {
				visible.WriteString(text)
			}
		}
		if special := geminiSpecialPartText(part); special != "" {
			messageContent = append(messageContent, map[string]any{"type": "output_text", "text": special})
		}
		if media := geminiMediaContentItem(part); media != nil {
			messageContent = append(messageContent, media)
		}
		if image := geminiImageGenerationItem(responseID, index, part); image != nil {
			imageItems = append(imageItems, image)
		}
		if rawCall, exists := part["functionCall"]; exists {
			call, _ := rawCall.(map[string]any)
			fallback := "call_gemini_" + uuid.NewString()
			if chatCompatible {
				fallback = "call_1"
			}
			toolItems = append(toolItems, geminiNativeToolCallItem(part, call, fallback))
		}
	}
	if visible.Len() > 0 {
		messageContent = append([]any{map[string]any{"type": "output_text", "text": visible.String()}}, messageContent...)
	}
	if len(messageContent) > 0 {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant", "content": messageContent,
		})
	}
	output = append(output, imageItems...)
	output = append(output, toolItems...)
	if grounding := geminiGroundingCall(root, responseID); grounding != nil {
		output = append(output, grounding)
	}
	if citations := geminiCitationText(root); citations != "" {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": citations}},
		})
	}

	result := map[string]any{
		"id": responseID, "object": "response", "model": model, "output": output,
	}
	if createdAt > 0 {
		result["created_at"] = createdAt
	}
	if rawUsage, present := root["usageMetadata"]; present {
		usage, _ := rawUsage.(map[string]any)
		result["usage"] = geminiNativeUsage(usage)
	}
	if metadata := geminiNativeResponseMetadataValue(root, candidate, requestMetadata); len(metadata) > 0 {
		result["metadata"] = metadata
	}
	geminiApplyNativeStatus(result, root, candidate, len(output) > 0)
	return result
}

func geminiNativeUsage(usage map[string]any) map[string]any {
	input := nativeUint(usage["promptTokenCount"])
	output := nativeUint(usage["candidatesTokenCount"])
	cached := nativeUint(usage["cachedContentTokenCount"])
	reasoning := nativeUint(usage["thoughtsTokenCount"])
	tool := nativeUint(usage["toolUsePromptTokenCount"])
	total, ok := nativeUintPresent(usage["totalTokenCount"])
	if !ok {
		if ^uint64(0)-input < output {
			total = ^uint64(0)
		} else {
			total = input + output
		}
	}
	return map[string]any{
		"input_tokens":          input,
		"input_tokens_details":  map[string]any{"cached_tokens": cached, "tool_tokens": tool},
		"output_tokens":         output,
		"output_tokens_details": map[string]any{"reasoning_tokens": reasoning},
		"total_tokens":          total,
	}
}

func nativeUint(value any) uint64 {
	converted, ok := nativeUintPresent(value)
	if !ok {
		return 0
	}
	return converted
}

func nativeUintPresent(value any) (uint64, bool) {
	switch value := value.(type) {
	case json.Number:
		// Tagged gemini_raw_u64 parses unsigned decimal *integer*
		// digits through all 64 bits. Int64() discards legitimate
		// [2^63,2^64-1] token usage, while Go float64 values correspond
		// to serde JSON numbers with a decimal point and are not valid.
		if parsed, err := strconv.ParseUint(string(value), 10, 64); err == nil {
			return parsed, true
		}
	case uint64:
		return value, true
	case int:
		if value >= 0 {
			return uint64(value), true
		}
	}
	return 0, false
}

func geminiNativeResponseMetadata(root, candidate map[string]any) map[string]any {
	result := make(map[string]any)
	for _, key := range []string{"promptFeedback", "usageMetadata"} {
		if value, ok := root[key]; ok && value != nil {
			result[key] = value
		}
	}
	for _, key := range []string{
		"finishReason", "finishMessage", "safetyRatings", "citationMetadata",
		"groundingMetadata", "urlContextMetadata", "avgLogprobs", "logprobsResult",
	} {
		if value, ok := candidate[key]; ok && value != nil {
			result[key] = value
		}
	}
	return result
}

func geminiApplyNativeStatus(result, root, candidate map[string]any, hasOutput bool) {
	if feedback, ok := root["promptFeedback"].(map[string]any); ok {
		if reason := nativeNonblank(feedback["blockReason"]); reason != "" {
			result["status"] = "failed"
			result["error"] = map[string]any{
				"code":    "gemini_prompt_blocked",
				"message": "Gemini blocked the prompt: " + reason,
			}
			return
		}
	}
	reason := nativeNonblank(candidate["finishReason"])
	switch reason {
	case "MAX_TOKENS":
		result["status"] = "incomplete"
		result["incomplete_details"] = geminiMaxTokensIncompleteDetails()
		return
	case "MALFORMED_FUNCTION_CALL":
		geminiNativeFailure(result, "gemini_malformed_function_call", reason)
		return
	case "UNEXPECTED_TOOL_CALL":
		geminiNativeFailure(result, "gemini_unexpected_tool_call", reason)
		return
	case "OTHER":
		geminiNativeFailure(result, "gemini_finish_other", reason)
		return
	case "NO_IMAGE":
		geminiNativeFailure(result, "gemini_no_image", reason)
		return
	case "SAFETY", "RECITATION", "LANGUAGE", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT":
		geminiNativeFailure(result, "invalid_prompt", reason)
		return
	}
	if !hasOutput {
		result["status"] = "failed"
		message := "Gemini returned no visible response content."
		if reason != "" {
			message += " finishReason=" + reason
		}
		result["error"] = map[string]any{"code": "gemini_empty_response", "message": message}
	}
}

func geminiNativeFailure(result map[string]any, code, reason string) {
	result["status"] = "failed"
	result["error"] = map[string]any{
		"code":    code,
		"message": "Gemini ended the stream with finishReason=" + reason,
	}
}

func geminiGroundingCall(root map[string]any, responseID string) map[string]any {
	candidate := firstNativeCandidate(root)
	if candidate == nil {
		return nil
	}
	sources := make([]map[string]any, 0)
	seen := make(map[string]bool)
	grounding, _ := candidate["groundingMetadata"].(map[string]any)
	if chunks, ok := grounding["groundingChunks"].([]any); ok {
		for _, raw := range chunks {
			chunk, _ := raw.(map[string]any)
			for _, kind := range []string{"web", "retrievedContext"} {
				source, _ := chunk[kind].(map[string]any)
				geminiAppendSource(&sources, seen, source)
			}
		}
	}
	citations, _ := candidate["citationMetadata"].(map[string]any)
	if array := firstNativeArray(citations, "citations", "citationSources"); array != nil {
		for _, raw := range array {
			source, _ := raw.(map[string]any)
			geminiAppendSource(&sources, seen, source)
		}
	}
	urlMetadata, _ := candidate["urlContextMetadata"].(map[string]any)
	if array := firstNativeArray(urlMetadata, "urlMetadata", "url_metadata"); array != nil {
		for _, raw := range array {
			source, _ := raw.(map[string]any)
			geminiAppendSource(&sources, seen, source)
		}
	}
	queries := make([]any, 0)
	if values, ok := grounding["webSearchQueries"].([]any); ok {
		for _, value := range values {
			if text, ok := value.(string); ok {
				queries = append(queries, text)
			}
		}
	}
	if len(sources) == 0 && len(queries) == 0 {
		return nil
	}
	sourceValues := make([]any, 0, len(sources))
	for _, source := range sources {
		sourceValues = append(sourceValues, source)
	}
	action := map[string]any{}
	if len(queries) > 0 {
		action["type"] = "search"
		action["queries"] = queries
		action["sources"] = sourceValues
	} else if len(sources) > 0 {
		action["type"] = "open_page"
		action["url"] = sources[0]["url"]
		action["sources"] = sourceValues
	}
	return map[string]any{
		"type": "web_search_call", "id": "ws_" + responseID,
		"status": "completed", "action": action,
	}
}

func firstNativeArray(object map[string]any, keys ...string) []any {
	for _, key := range keys {
		if object == nil {
			return nil
		}
		if value, exists := object[key]; exists {
			array, _ := value.([]any)
			return array
		}
	}
	return nil
}

func geminiAppendSource(result *[]map[string]any, seen map[string]bool, source map[string]any) {
	if source == nil {
		return
	}
	url, present := geminiAliasString(source, "uri", "url", "retrievedUrl", "retrieved_url")
	if !present || strings.TrimSpace(url) == "" || seen[url] {
		return
	}
	item := map[string]any{"type": "url", "url": url}
	if title, present := geminiAliasString(source, "title"); present && strings.TrimSpace(title) != "" {
		item["title"] = title
	}
	for _, key := range []string{"urlRetrievalStatus", "url_retrieval_status", "status"} {
		// Tagged gemini_grounding_source_status uses first PRESENT field,
		// even when it is null. A null primary explicitly suppresses
		// all lower-priority status aliases.
		if status, present := source[key]; present {
			if status != nil {
				item["status"] = status
			}
			break
		}
	}
	seen[url] = true
	*result = append(*result, item)
}

func geminiAliasString(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if ok {
			return text, true
		}
	}
	return "", false
}

func geminiCitationText(root map[string]any) string {
	candidate := firstNativeCandidate(root)
	if candidate == nil || nativeNonblank(candidate["finishReason"]) == "" {
		return ""
	}
	metadata, _ := candidate["citationMetadata"].(map[string]any)
	citations, _ := metadata["citations"].([]any)
	lines := make([]string, 0, len(citations))
	seen := make(map[string]bool)
	for _, raw := range citations {
		citation, _ := raw.(map[string]any)
		url, present := geminiAliasString(citation, "uri")
		if !present || strings.TrimSpace(url) == "" {
			continue
		}
		line := url
		if title, present := geminiAliasString(citation, "title"); present && strings.TrimSpace(title) != "" {
			line = fmt.Sprintf("(%s) %s", title, url)
		}
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return "Citations:\n" + strings.Join(lines, "\n")
}

func geminiImageGenerationItem(responseID string, index int, part map[string]any) map[string]any {
	inline, _ := firstPresentNative(part, "inlineData", "inline_data").(map[string]any)
	if inline == nil {
		return nil
	}
	mime := nativeString(firstPresentNative(inline, "mimeType", "mime_type"))
	data, dataIsString := inline["data"].(string)
	if !strings.HasPrefix(mime, "image/") || !dataIsString {
		return nil
	}
	return map[string]any{
		"type":   "image_generation_call",
		"id":     fmt.Sprintf("ig_%s_%d", responseID, index),
		"status": "completed", "result": data,
	}
}
