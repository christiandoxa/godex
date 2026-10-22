package main

func deepSeekBridgeMessageSuffixes04384() []string {
	return []string{"bridge-message-normal-text", "bridge-message-normal-paired", "bridge-message-normal-unpaired", "bridge-message-normal-unmatched", "bridge-message-normal-null", "bridge-message-normal-object", "bridge-message-simple-paired"}
}

func deepSeekBridgeMessageFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	argument, outputID := `"{}"`, "call_synthetic"
	paired, calls := true, true
	inputPrefix := `{"type":"message","role":"developer","content":"synthetic instruction"},`
	wirePrefix := `{"content":"synthetic instruction","role":"system"},`
	switch suffix {
	case "bridge-message-normal-text":
		paired, calls = false, false
	case "bridge-message-normal-paired":
	case "bridge-message-normal-unpaired":
		paired = false
	case "bridge-message-normal-unmatched":
		outputID = "call_other"
	case "bridge-message-normal-null":
		argument = `null`
	case "bridge-message-normal-object":
		argument = `{"x":1}`
	case "bridge-message-simple-paired":
		inputPrefix, wirePrefix = "", ""
	default:
		return requestPrimitiveFixture04384{}, false
	}
	callField := `,"tool_calls":[{"id":"call_synthetic","type":"function","function":{"name":"lookup","arguments":` + argument + `}}]`
	if !calls {
		callField = ""
	}
	output := `,{"type":"function_call_output","call_id":"` + outputID + `","output":"done"}`
	if !paired {
		output = ""
	}
	body := `{"model":"gemini-3.1-pro-preview","input":[` + inputPrefix + `{"type":"message","role":"assistant","content":"preface"` + callField + `}` + output + `,{"type":"message","role":"user","content":"same request"}]}`
	wireCalls, wireOutput := "", ""
	if suffix == "bridge-message-simple-paired" {
		wireCalls = `,"tool_calls":[{"function":{"arguments":"{}","name":"lookup"},"id":"call_synthetic","type":"function"}]`
		wireOutput = `,{"content":"done","role":"tool","tool_call_id":"call_synthetic"}`
	}
	wire := `{"messages":[` + wirePrefix + `{"content":"preface","role":"assistant"` + wireCalls + `}` + wireOutput + `,{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return requestPrimitiveFixture04384{body: body, wireBody: wire}, true
}
