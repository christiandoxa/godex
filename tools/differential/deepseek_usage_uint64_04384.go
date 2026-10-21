package main

import (
	"encoding/json"
	"reflect"
	"strings"
)

const deepSeekUnsignedUsageInput04384 = `{"prompt_tokens":18446744073709551615,"completion_tokens":9007199254740993,"prompt_cache_hit_tokens":9223372036854775808,"prompt_cache_miss_tokens":18446744073709551615,"completion_tokens_details":{"reasoning_tokens":18446744073709551615}}`
const deepSeekUnsignedUsageOutput04384 = `{"input_tokens":18446744073709551615,"output_tokens":9007199254740993,"total_tokens":18446744073709551615,"input_tokens_details":{"cached_tokens":9223372036854775808},"output_tokens_details":{"reasoning_tokens":18446744073709551615},"metadata":{"deepseek":{"prompt_cache_hit_tokens":9223372036854775808,"prompt_cache_miss_tokens":18446744073709551615}}}`

// Check every integer token before applying the existing ordinary fixture oracle.
// Its float64 decoder must never decide whether these large values are equal.
func normalizeDeepSeekUnsignedUsage04384(response map[string]any) bool {
	var expected any
	decoder := json.NewDecoder(strings.NewReader(deepSeekUnsignedUsageOutput04384))
	decoder.UseNumber()
	if decoder.Decode(&expected) != nil || !reflect.DeepEqual(response["usage"], expected) {
		return false
	}
	response["usage"] = map[string]any{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3}
	return true
}

func validDeepSeekUnsignedUsage04384(body string, stream bool) bool {
	if stream {
		_, ok := canonicalDeepSeekUnsignedSSE04384(body)
		return ok
	}
	if !json.Valid([]byte(body)) {
		return false
	}
	var response map[string]any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&response) != nil || response["created_at"] != json.Number("18446744073709551615") ||
		!normalizeDeepSeekUnsignedUsage04384(response) {
		return false
	}
	encoded, err := json.Marshal(response)
	return err == nil && validFixtureResponse(string(encoded))
}

func canonicalDeepSeekUnsignedSSE04384(body string) (string, bool) {
	if !strings.HasSuffix(body, "\r\n\r\n") {
		return "", false
	}
	var normalized strings.Builder
	for _, block := range strings.Split(strings.TrimSuffix(body, "\r\n\r\n"), "\r\n\r\n") {
		lines := strings.Split(block, "\r\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[1], "data: ") {
			return "", false
		}
		data := strings.TrimPrefix(lines[1], "data: ")
		if !json.Valid([]byte(data)) {
			return "", false
		}
		var event map[string]any
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&event) != nil {
			return "", false
		}
		if epoch, present := event["created_at"]; present {
			if epoch != json.Number("18446744073709551615") {
				return "", false
			}
			event["created_at"] = 1
		}
		if event["type"] == "response.completed" {
			response, _ := event["response"].(map[string]any)
			if !normalizeDeepSeekUnsignedUsage04384(response) {
				return "", false
			}
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return "", false
		}
		normalized.WriteString(lines[0] + "\r\ndata: " + string(encoded) + "\r\n\r\n")
	}
	canonical, err := canonicalFixtureSSE(normalized.String())
	return canonical, err == nil
}

func equivalentDeepSeekUnsignedSSE04384(left, right string) bool {
	a, leftOK := canonicalDeepSeekUnsignedSSE04384(left)
	b, rightOK := canonicalDeepSeekUnsignedSSE04384(right)
	return leftOK && rightOK && a == b
}
