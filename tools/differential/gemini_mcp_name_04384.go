package main

func geminiMCPNameSuffixes04384() []string {
	return []string{"mcp-name-null", "mcp-name-number", "mcp-name-sentinel", "mcp-name-nested-control", "mcp-name-valid-control", "mcp-name-inline-orphan",
		"mcp-name-inline-object-control", "mcp-name-inline-null-control", "mcp-name-earlier-object", "mcp-name-earlier-array", "mcp-name-earlier-false", "mcp-name-earlier-no-result",
		"mcp-name-earlier-content", "mcp-name-earlier-result", "mcp-name-earlier-error", "mcp-name-implicit-tool-control"}
}

func geminiMCPNameFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	name, inline := `"name":null,"tool_name":"synthetic_lookup"`, ""
	result := `"synthetic-result"`
	empty, earlier, external := true, false, true
	switch suffix {
	case "mcp-name-null":
	case "mcp-name-number":
		name = `"name":7,"function":{"name":"synthetic_lookup"}`
	case "mcp-name-sentinel":
		name = `"__missing__":"synthetic_lookup"`
	case "mcp-name-nested-control":
		name, empty = `"function":{"name":"synthetic_lookup"}`, false
	case "mcp-name-valid-control":
		name, empty = `"name":"synthetic_lookup"`, false
	case "mcp-name-inline-orphan":
		inline, external = `,"output":"synthetic-result"`, false
	case "mcp-name-inline-object-control", "mcp-name-earlier-object":
		inline, result, external = `,"output":{"b":1.00,"a":"<","large":9007199254740993}`, `{"a":"<","b":1.0,"large":9007199254740993}`, false
		if suffix == "mcp-name-inline-object-control" {
			name, empty = `"name":"synthetic_lookup"`, false
		} else {
			earlier = true
		}
	case "mcp-name-inline-null-control":
		name, inline, result, empty, external = `"name":"synthetic_lookup"`, `,"output":null`, "null", false, false
	case "mcp-name-earlier-array":
		inline, result, earlier, external = `,"output":[{"text":"part"},null]`, `[{"text":"part"},null]`, true, false
	case "mcp-name-earlier-false":
		inline, result, earlier, external = `,"output":false,"content":"ignored"`, "false", true, false
	case "mcp-name-earlier-no-result":
		earlier, external, result = true, false, ""
	case "mcp-name-earlier-content":
		inline, result, earlier, external = `,"content":{"value":1}`, `{"value":1}`, true, false
	case "mcp-name-earlier-result":
		inline, result, earlier, external = `,"result":[null,false]`, `[null,false]`, true, false
	case "mcp-name-earlier-error":
		inline, result, earlier, external = `,"error":false`, "false", true, false
	case "mcp-name-implicit-tool-control":
		return requestPrimitiveFixture04384{
			body:     `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"role":"tool","tool_call_id":"synthetic-call","content":"keep"},{"type":"function_call","call_id":"synthetic-call","name":"first_lookup","arguments":"{}"}]}`,
			wireBody: `{"messages":[{"content":"same request","role":"user"},{"content":"keep","role":"tool"}],"model":"gemini-3.1-pro-preview","stream":false}`,
		}, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	first := ""
	if earlier {
		first = `{"type":"function_call","call_id":"synthetic-call","name":"first_lookup","arguments":"{}","thought_signature":"first-signature"},`
	}
	output := ""
	if external {
		output = `,{"type":"function_call_output","call_id":"synthetic-call","output":"synthetic-result"}`
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},` + first + `{"type":"mcp_call","call_id":"synthetic-call",` + name + `,"arguments":"{}"` + inline + `}` + output + `]}`
	messages := `{"content":"same request","role":"user"}`
	if earlier && inline != "" {
		messages += `,{"content":"","role":"assistant","tool_calls":[{"extra_content":{"google":{"thought_signature":"first-signature"}},"function":{"arguments":"{}","name":"first_lookup"},"id":"synthetic-call","type":"function"}]},{"content":` + result + `,"role":"tool","tool_call_id":"synthetic-call"}`
	}
	if empty {
		messages += `,{"content":"","role":"assistant"}`
	} else {
		messages += `,{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"synthetic_lookup"},"id":"synthetic-call","type":"function"}]},{"content":` + result + `,"role":"tool","tool_call_id":"synthetic-call"}`
	}
	return requestPrimitiveFixture04384{body: body, wireBody: `{"messages":[` + messages + `],"model":"gemini-3.1-pro-preview","stream":false}`}, true
}
