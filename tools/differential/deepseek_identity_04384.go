package main

import (
	"encoding/json"
	"strings"
)

func normalizeDeepSeekEmptyIdentity04384(body string, stream bool) (string, bool) {
	wantIDs, wantReferences, id := 1, 0, "chatcmpl-differential"
	if stream {
		wantIDs, wantReferences, id = 2, 3, syntheticResponseID
	}
	if strings.Count(body, `"id":""`) != wantIDs || strings.Count(body, `"response_id":""`) != wantReferences ||
		strings.Count(body, `"model":""`) != 1 {
		return "", false
	}
	body = strings.ReplaceAll(body, `"id":""`, `"id":"`+id+`"`)
	body = strings.ReplaceAll(body, `"response_id":""`, `"response_id":"`+id+`"`)
	return strings.Replace(body, `"model":""`, `"model":"deepseek-v4-pro"`, 1), true
}

func validDeepSeekEmptyIdentity04384(body string, stream bool) bool {
	if !stream {
		var response map[string]json.RawMessage
		if json.Unmarshal([]byte(body), &response) != nil || string(response["id"]) != `""` || string(response["model"]) != `""` {
			return false
		}
	}
	normalized, ok := normalizeDeepSeekEmptyIdentity04384(body, stream)
	if !ok {
		return false
	}
	if stream {
		return validFixtureSSE(normalized)
	}
	return validFixtureResponse(normalized)
}

func equivalentDeepSeekEmptyIdentitySSE04384(left, right string) bool {
	a, leftOK := normalizeDeepSeekEmptyIdentity04384(left, true)
	b, rightOK := normalizeDeepSeekEmptyIdentity04384(right, true)
	return leftOK && rightOK && equivalentFixtureSSE(a, b)
}
