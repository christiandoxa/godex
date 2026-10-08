package gemini

func geminiNativeResponseMetadataValue(
	root, candidate, requestMetadata map[string]any,
) map[string]any {
	metadata := make(map[string]any)
	mergeMetadataFields(metadata, requestMetadata)
	if provider := geminiNativeResponseMetadata(root, candidate); len(provider) > 0 {
		existing, _ := metadata["gemini"].(map[string]any)
		merged := make(map[string]any)
		mergeMetadataFields(merged, existing)
		mergeMetadataFields(merged, provider)
		metadata["gemini"] = merged
	}
	return metadata
}
