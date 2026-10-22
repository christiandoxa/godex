package main

func geminiOrphanOutputSuffixes04384() []string {
	return []string{"orphan-output-null", "orphan-output-empty", "orphan-output-string", "orphan-output-custom", "orphan-output-mcp", "orphan-output-control", "orphan-output-only"}
}

func geminiOrphanOutputFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	kind, output := "function_call_output", "null"
	switch suffix {
	case "orphan-output-null":
	case "orphan-output-empty":
		output = `""`
	case "orphan-output-string":
		output = `"synthetic-result"`
	case "orphan-output-custom":
		kind = "custom_tool_call_output"
	case "orphan-output-mcp":
		kind = "mcp_tool_result"
	case "orphan-output-control":
		return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"}]}`}, true
	case "orphan-output-only":
		return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":[{"type":"function_call_output","call_id":"synthetic-call","output":null}]}`,
			wireBody: `{"messages":[{"content":"","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`}, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"` + kind + `","call_id":"synthetic-call","output":` + output + `}]}`}, true
}
