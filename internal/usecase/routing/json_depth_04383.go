package routing

// serde_json 1.0.151, pinned by Prodex 0.438.3, rejects the 128th nested
// object/array, even when the compiled Mojo scanner could handle more nodes.
// The Go JSON parser accepts deeper values; reject them before semantic use.
func validNativeJSONContainerDepth04383(value any) bool {
	return validNativeJSONContainerAtDepth04383(value, 0)
}

func validNativeJSONContainerAtDepth04383(value any, parentDepth int) bool {
	switch typed := value.(type) {
	case map[string]any:
		if parentDepth >= 127 {
			return false
		}
		for _, member := range typed {
			if !validNativeJSONContainerAtDepth04383(member, parentDepth+1) {
				return false
			}
		}
	case []any:
		if parentDepth >= 127 {
			return false
		}
		for _, item := range typed {
			if !validNativeJSONContainerAtDepth04383(item, parentDepth+1) {
				return false
			}
		}
	}
	return true
}
