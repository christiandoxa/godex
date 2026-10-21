package main

import (
	"encoding/json"
	"strings"
)

func streamToolNameFixture04384(scenario string) (name string, known bool) {
	switch scenario {
	case "sse-tool-name-padded":
		return " tool_search ", true
	case "sse-tool-name-blank-namespace":
		return "ns-- ", true
	case "sse-tool-name-blank-late":
		return "", true
	}
	return "", false
}

func streamToolNameWire04384(scenario string) string {
	name, known := streamToolNameFixture04384(scenario)
	if !known {
		return ""
	}
	signature, _ := json.Marshal(streamedToolSignature04384)
	wire := strings.ReplaceAll(canonicalToolSignatureWire04384(), `,"gemini_thought_signature":`+string(signature), "")
	if scenario == "sse-tool-name-blank-late" {
		prefix := strings.SplitN(wire, "event: response.output_item.done", 2)[0]
		return prefix + "event: response.failed\r\ndata: " + `{"created_at":1,"response":{"error":{"code":"invalid_tool_call_arguments","message":"DeepSeek streamed a tool call without a function name at index 0"},"id":"chatcmpl-toolargs"},"sequence_number":3,"type":"response.failed"}` + "\r\n\r\n"
	}
	quoted, _ := json.Marshal(name)
	return strings.ReplaceAll(wire, `"name":"shell"`, `"name":`+string(quoted))
}

func equivalentStreamToolNameSSE04384(left, right string) bool {
	if left != right {
		return false
	}
	for _, scenario := range []string{"sse-tool-name-padded", "sse-tool-name-blank-namespace", "sse-tool-name-blank-late"} {
		if left == streamToolNameWire04384(scenario) {
			return true
		}
	}
	return false
}
