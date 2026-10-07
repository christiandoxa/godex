package gemini

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

func geminiNativeResponsesValue(root map[string]any, requestMetadata map[string]any, createdAt int64) map[string]any {
	responseID := nativeString(root["responseId"])
	if responseID == "" {
		responseID = nativeString(root["id"])
	}
	if responseID == "" {
		responseID = "resp_gemini_" + uuid.NewString()
	}
	model := nativeString(root["modelVersion"])
	if model == "" {
		model = nativeString(root["model"])
	}
	if model == "" {
		model = "auto"
	}

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
		if call, ok := part["functionCall"].(map[string]any); ok {
			toolItems = append(toolItems, geminiNativeToolCallItem(
				part, call, "call_gemini_"+uuid.NewString(),
			))
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
	metadata := make(map[string]any)
	mergeMetadataFields(metadata, requestMetadata)
	if provider := geminiNativeResponseMetadata(root, candidate); len(provider) > 0 {
		existing, _ := metadata["gemini"].(map[string]any)
		merged := make(map[string]any)
		mergeMetadataFields(merged, existing)
		mergeMetadataFields(merged, provider)
		metadata["gemini"] = merged
	}
	result["metadata"] = metadata
	geminiApplyNativeStatus(result, root, candidate, len(output) > 0)
	return result
}

func firstNativeCandidate(root map[string]any) map[string]any {
	candidates, _ := root["candidates"].([]any)
	if len(candidates) == 0 {
		return nil
	}
	candidate, _ := candidates[0].(map[string]any)
	return candidate
}

func nativeCandidateParts(candidate map[string]any) []any {
	if candidate == nil {
		return nil
	}
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	return parts
}

func nativeString(value any) string {
	text, _ := value.(string)
	return text
}

func nativeNonblank(value any) string {
	text := nativeString(value)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

func firstPresentNative(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if object != nil {
			if value, ok := object[key]; ok {
				return value
			}
		}
	}
	return nil
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
		parsed, err := value.Int64()
		if err == nil && parsed >= 0 {
			return uint64(parsed), true
		}
	case float64:
		if value >= 0 && value == float64(uint64(value)) {
			return uint64(value), true
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
		if status, ok := source[key]; ok && status != nil {
			item["status"] = status
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

func geminiSpecialPartText(part map[string]any) string {
	if code, ok := part["executableCode"].(map[string]any); ok {
		language := nativeString(code["language"])
		if language == "" {
			language = "text"
		}
		body := nativeString(code["code"])
		if strings.TrimSpace(body) == "" {
			return ""
		}
		return fmt.Sprintf("Gemini executable code (%s):\n```%s\n%s\n```", language, language, body)
	}
	if result, ok := part["codeExecutionResult"].(map[string]any); ok {
		outcome := nativeString(result["outcome"])
		if outcome == "" {
			outcome = "OUTCOME_UNSPECIFIED"
		}
		return fmt.Sprintf("Gemini code execution result (%s):\n```text\n%s\n```", outcome, nativeString(result["output"]))
	}
	if video, ok := part["videoMetadata"]; ok {
		encoded, _ := json.Marshal(video)
		return "Gemini video metadata: " + string(encoded)
	}
	return ""
}

func geminiMediaContentItem(part map[string]any) map[string]any {
	inline, _ := firstPresentNative(part, "inlineData", "inline_data").(map[string]any)
	if inline != nil {
		mime := nativeString(firstPresentNative(inline, "mimeType", "mime_type"))
		if mime == "" {
			mime = "application/octet-stream"
		}
		data, _ := inline["data"].(string)
		if data == "" {
			return nil
		}
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": "data:" + mime + ";base64," + data}
		}
		return map[string]any{"type": "output_text", "text": fmt.Sprintf("Gemini returned inline %s media (%d base64 characters).", mime, len(data))}
	}
	file, _ := firstPresentNative(part, "fileData", "file_data").(map[string]any)
	if file != nil {
		uri := nativeString(firstPresentNative(file, "fileUri", "file_uri"))
		if uri == "" {
			return nil
		}
		mime := nativeString(firstPresentNative(file, "mimeType", "mime_type"))
		if mime == "" {
			mime = geminiMIMEForURI(uri)
		}
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": uri}
		}
		return map[string]any{"type": "output_text", "text": "Gemini returned " + mime + " media: " + uri}
	}
	if text, _ := part["text"].(string); strings.HasPrefix(text, "data:") {
		meta, body, ok := strings.Cut(strings.TrimPrefix(text, "data:"), ",")
		if !ok || !strings.HasSuffix(meta, ";base64") {
			return nil
		}
		mime := strings.TrimSuffix(meta, ";base64")
		if strings.HasPrefix(mime, "image/") {
			return map[string]any{"type": "input_image", "image_url": text}
		}
		return map[string]any{"type": "output_text", "text": fmt.Sprintf("Gemini returned inline %s media (%d base64 characters).", mime, len(body))}
	}
	return nil
}

func geminiImageGenerationItem(responseID string, index int, part map[string]any) map[string]any {
	inline, _ := firstPresentNative(part, "inlineData", "inline_data").(map[string]any)
	if inline == nil {
		return nil
	}
	mime := nativeString(firstPresentNative(inline, "mimeType", "mime_type"))
	data, _ := inline["data"].(string)
	if !strings.HasPrefix(mime, "image/") || data == "" {
		return nil
	}
	return map[string]any{
		"type":   "image_generation_call",
		"id":     fmt.Sprintf("ig_%s_%d", responseID, index),
		"status": "completed", "result": data,
	}
}

func geminiMIMEForURI(uri string) string {
	lower := strings.ToLower(uri)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mov"):
		return "video/quicktime"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
