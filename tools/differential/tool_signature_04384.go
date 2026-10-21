package main

import "encoding/json"

const streamedToolSignature04384 = "\t sig_🧭 \n"

func canonicalToolSignatureWire04384() string {
	signature, _ := json.Marshal(streamedToolSignature04384)
	item := `{"arguments":"{}","call_id":"call-toolargs","gemini_thought_signature":` + string(signature) + `,"name":"shell","type":"function_call"}`
	response := `{"id":"chatcmpl-toolargs","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","output":[` + item + `]}`
	return "event: response.created\r\ndata: " + `{"created_at":1,"response":{"id":"chatcmpl-toolargs"},"sequence_number":0,"type":"response.created"}` + "\r\n\r\n" +
		"event: response.output_item.added\r\ndata: " + `{"item":{"call_id":"call-toolargs","name":"shell","type":"function_call"},"output_index":0,"sequence_number":1,"type":"response.output_item.added"}` + "\r\n\r\n" +
		"event: response.function_call_arguments.delta\r\ndata: " + `{"call_id":"call-toolargs","delta":"{}","output_index":0,"sequence_number":2,"type":"response.function_call_arguments.delta"}` + "\r\n\r\n" +
		"event: response.output_item.done\r\ndata: " + `{"item":` + item + `,"output_index":0,"sequence_number":3,"type":"response.output_item.done"}` + "\r\n\r\n" +
		"event: response.completed\r\ndata: " + `{"created_at":1,"response":` + response + `,"sequence_number":4,"type":"response.completed"}` + "\r\n\r\n"
}

func equivalentToolSignatureSSE04384(left, right string) bool {
	return left == right && left == canonicalToolSignatureWire04384()
}
