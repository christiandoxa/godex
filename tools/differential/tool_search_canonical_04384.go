package main

import "strings"

const toolSearchRawArguments04384 = `{"max":18446744073709551615,"large":9007199254740993,"decimal":1e2,"neg":-0,"array":[18446744073709551615],"null":null}`
const toolSearchCanonicalArguments04384 = `{"array":[18446744073709551615],"decimal":100.0,"large":9007199254740993,"max":18446744073709551615,"neg":-0.0,"null":null}`

func canonicalToolSearchWire04384(stream bool) string {
	id, call := "chatcmpl-differential-tool", "call-differential"
	if stream {
		id, call = "chatcmpl-toolargs", "call-toolargs"
	}
	item := `{"arguments":` + toolSearchCanonicalArguments04384 + `,"call_id":"` + call + `","execution":"client","type":"tool_search_call"}`
	response := `{"id":"` + id + `","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","output":[` + item + `]}`
	if !stream {
		response = strings.Replace(response, `{"id":`, `{"created_at":1,"id":`, 1)
		response = strings.Replace(response, `,"output":`, `,"object":"response","output":`, 1)
		return strings.TrimSuffix(response, "}") + `,"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	}
	return "event: response.created\r\ndata: " + `{"created_at":1,"response":{"id":"` + id + `"},"sequence_number":0,"type":"response.created"}` + "\r\n\r\n" +
		"event: response.output_item.done\r\ndata: " + `{"item":` + item + `,"output_index":0,"sequence_number":1,"type":"response.output_item.done"}` + "\r\n\r\n" +
		"event: response.completed\r\ndata: " + `{"created_at":1,"response":` + response + `,"sequence_number":2,"type":"response.completed"}` + "\r\n\r\n"
}

func equivalentCanonicalToolSearchSSE04384(left, right string) bool {
	return left == right && left == canonicalToolSearchWire04384(true)
}
