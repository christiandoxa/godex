package deepseek

// The compiled Prodex 0.438.0 Anthropic RequestKernel rejects a JSON value
// whose child recursion exceeds ANTHROPIC_REQUEST_KERNEL_MAX_DEPTH=128.
// The native response translator passes only the content-block array to
// that kernel, NOT the outer provider response metadata. Check the parsed
// value tree with the same root-array depth zero, before constructing
// output. This avoids serializing or traversing unbounded nested objects.
const maxAnthropicResponseJSONDepth04380 = 128

func validAnthropicResponseJSONDepth04380(value any, depth int) bool {
	if depth > maxAnthropicResponseJSONDepth04380 {
		return false
	}
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if !validAnthropicResponseJSONDepth04380(child, depth+1) {
				return false
			}
		}
	case map[string]any:
		for _, child := range typed {
			if !validAnthropicResponseJSONDepth04380(child, depth+1) {
				return false
			}
		}
	}
	return true
}
