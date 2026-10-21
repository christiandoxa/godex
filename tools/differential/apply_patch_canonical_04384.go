package main

import "encoding/json"

func canonicalApplyPatchWire04384() string {
	input, _ := json.Marshal(toolSearchCanonicalArguments04384)
	item := `{"call_id":"call-toolargs","input":` + string(input) + `,"name":"apply_patch","type":"custom_tool_call"}`
	response := `{"id":"chatcmpl-toolargs","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","output":[` + item + `]}`
	return "event: response.created\r\ndata: " + `{"created_at":1,"response":{"id":"chatcmpl-toolargs"},"sequence_number":0,"type":"response.created"}` + "\r\n\r\n" +
		"event: response.output_item.added\r\ndata: " + `{"item":{"call_id":"call-toolargs","input":"","name":"apply_patch","type":"custom_tool_call"},"output_index":0,"sequence_number":1,"type":"response.output_item.added"}` + "\r\n\r\n" +
		"event: response.output_item.done\r\ndata: " + `{"item":` + item + `,"output_index":0,"sequence_number":2,"type":"response.output_item.done"}` + "\r\n\r\n" +
		"event: response.completed\r\ndata: " + `{"created_at":1,"response":` + response + `,"sequence_number":3,"type":"response.completed"}` + "\r\n\r\n"
}

func equivalentCanonicalApplyPatchSSE04384(left, right string) bool {
	return left == right && left == canonicalApplyPatchWire04384()
}
