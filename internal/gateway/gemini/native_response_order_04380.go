package gemini

import "sort"

// Tagged 0.438.0 operation 4 delegates to the existing
// gemini_tool_response_order_plan ABI. Only an immediate model→user pair
// with at least one functionCall is eligible. Eligible user parts sort in
// three stable groups: responses with a call ID match (by first call index),
// unmatched responses, then every other part in its original order. Names
// are deliberately *not* part of this ordering key (unlike pair matching).
func geminiRefineToolResponseOrder04380(contents []any) []any {
	result := make([]any, 0, len(contents))
	for index, current := range contents {
		if index == 0 || !geminiToolRole04380(current, "user") || !geminiToolRole04380(contents[index-1], "model") {
			result = append(result, current)
			continue
		}
		calls := geminiToolValues04380(contents[index-1], "functionCall")
		parts, valid := geminiContentParts04380(current)
		if !valid || len(calls) == 0 || len(parts) < 2 {
			result = append(result, current)
			continue
		}
		ids := make([]string, 0, len(calls))
		for _, call := range calls {
			ids = append(ids, geminiToolIdentity04380(call, "id"))
		}
		ordered := append([]any(nil), parts...)
		sort.SliceStable(ordered, func(left, right int) bool {
			aPresent, aRank := geminiResponseOrderKey04380(ordered[left], ids)
			bPresent, bRank := geminiResponseOrderKey04380(ordered[right], ids)
			if aPresent != bPresent {
				return aPresent
			}
			return aPresent && aRank < bRank
		})
		copied := make(map[string]any, len(current.(map[string]any)))
		for key, value := range current.(map[string]any) {
			copied[key] = value
		}
		copied["parts"] = ordered
		result = append(result, copied)
	}
	return result
}

func geminiResponseOrderKey04380(raw any, callIDs []string) (bool, int) {
	part, ok := raw.(map[string]any)
	if !ok {
		return false, 0
	}
	value, present := part["functionResponse"]
	if !present {
		return false, 0
	}
	id := geminiToolIdentity04380(value, "id")
	for rank, expected := range callIDs {
		if id == expected {
			return true, rank
		}
	}
	return true, len(callIDs)
}
