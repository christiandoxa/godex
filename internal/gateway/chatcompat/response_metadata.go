package chatcompat

func chatResponseMetadata(root, choice, message map[string]any) map[string]any {
	metadata := make(map[string]any)
	appendChoiceMetadata(metadata, choice)
	appendMessageMetadata(metadata, message)
	appendStringMetadata(metadata, root, "system_fingerprint")
	return metadata
}

func appendChoiceMetadata(metadata, choice map[string]any) {
	if choice == nil {
		return
	}
	if value, ok := choice["logprobs"]; ok && value != nil {
		metadata["logprobs"] = value
	}
	appendStringMetadata(metadata, choice, "finish_reason")
}

func appendMessageMetadata(metadata, message map[string]any) {
	if message == nil {
		return
	}
	for _, key := range []string{"reasoning_content", "refusal"} {
		appendStringMetadata(metadata, message, key)
	}
	if annotations, ok := message["annotations"].([]any); ok && len(annotations) > 0 {
		metadata["annotations"] = annotations
	}
}

func appendStringMetadata(target, source map[string]any, key string) {
	if value, ok := source[key].(string); ok && value != "" {
		target[key] = value
	}
}

func chatUsage(value any, providerKey string) map[string]any {
	usage, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	input := uintValueOr(usage["prompt_tokens"], 0)
	output := uintValueOr(usage["completion_tokens"], 0)
	total := uintValueOr(usage["total_tokens"], input+output)
	result := map[string]any{"input_tokens": input, "output_tokens": output, "total_tokens": total}
	cacheHit, hitFound := uintOptional(usage["prompt_cache_hit_tokens"])
	cacheMiss, missFound := uintOptional(usage["prompt_cache_miss_tokens"])
	if hitFound {
		result["input_tokens_details"] = map[string]any{"cached_tokens": cacheHit}
	}
	if details, ok := usage["completion_tokens_details"].(map[string]any); ok {
		if reasoning, found := uintOptional(details["reasoning_tokens"]); found {
			result["output_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}
		}
	}
	if providerKey != "" && (hitFound || missFound) {
		result["metadata"] = map[string]any{providerKey: map[string]any{
			"prompt_cache_hit_tokens":  cacheHit,
			"prompt_cache_miss_tokens": cacheMiss,
		}}
	}
	return result
}

func uintOptional(value any) (uint64, bool) {
	if value == nil {
		return 0, false
	}
	converted := uintValueOr(value, ^uint64(0))
	if converted == ^uint64(0) {
		return 0, false
	}
	return converted, true
}
