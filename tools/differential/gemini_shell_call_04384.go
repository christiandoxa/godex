package main

func geminiShellCallSuffixes04384() []string {
	return []string{"shell-call-control", "shell-call-action-array", "shell-call-null-id", "shell-call-top-array", "shell-call-nested-string", "shell-call-nested-empty", "shell-call-nested-number", "shell-call-nested-blank",
		"shell-call-root-wins", "shell-call-root-null", "shell-call-root-mixed", "shell-call-invalid-action-shadow", "shell-call-missing-action-command", "shell-call-action-scalar", "shell-call-option-null", "shell-call-options", "shell-call-unicode", "shell-call-signature-ignored"}
}

func geminiShellCallFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fields, arguments := `"command":"echo synthetic"`, `{\"command\":\"echo synthetic\"}`
	identity, wireID := `"call_id":"synthetic-call"`, "synthetic-call"
	fixture := requestPrimitiveFixture04384{}
	switch suffix {
	case "shell-call-control":
	case "shell-call-action-array":
		fields = `"action":{"command":["echo","synthetic"]}`
	case "shell-call-null-id":
		identity, wireID = `"call_id":null,"tool_call_id":"synthetic-call"`, "call_1"
	case "shell-call-top-array":
		fields, fixture.reject = `"command":["echo","synthetic"]`, true
	case "shell-call-nested-string":
		fields, fixture.reject = `"action":{"command":"echo synthetic"}`, true
	case "shell-call-nested-empty":
		fields, fixture.reject = `"action":{"command":[]}`, true
	case "shell-call-nested-number":
		fields, fixture.reject = `"action":{"command":["echo",7]}`, true
	case "shell-call-nested-blank":
		fields, fixture.reject = `"action":{"command":["echo"," \t"]}`, true
	case "shell-call-root-wins":
		fields = `"command":"echo synthetic","action":{"command":["ignored"]}`
	case "shell-call-root-null":
		fields, arguments = `"command":null,"action":{"command":["echo","synthetic"]}`, `{\"command\":\"\"}`
	case "shell-call-root-mixed":
		fields = `"command":["echo",7,"synthetic",null],"action":{"command":["valid"]}`
	case "shell-call-invalid-action-shadow":
		fields, fixture.reject = `"command":"echo synthetic","action":{"command":"ignored"}`, true
	case "shell-call-missing-action-command":
		fields, fixture.reject = `"command":"echo synthetic","action":{"cwd":"ignored"}`, true
	case "shell-call-action-scalar":
		fields += `,"action":null`
	case "shell-call-option-null":
		fields, arguments = `"command":"echo synthetic","cwd":null,"action":{"command":["valid"],"cwd":"ignored"}`, `{\"command\":\"echo synthetic\",\"cwd\":null}`
	case "shell-call-options":
		fields += `,"cwd":"<synthetic>","timeout":1.00,"env":{"b":9007199254740993,"a":"<&"}`
		arguments = `{\"command\":\"echo synthetic\",\"cwd\":\"<synthetic>\",\"timeout\":1.0,\"env\":{\"a\":\"<&\",\"b\":9007199254740993}}`
	case "shell-call-unicode":
		fields, arguments = `"command":"echo <&🌍"`, `{\"command\":\"echo <&🌍\"}`
	case "shell-call-signature-ignored":
		fields += `,"thought_signature":"ignored","name":"ignored"`
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"same request"},{"type":"local_shell_call",` + identity + `,` + fields + `},{"type":"function_call_output",` + identity + `,"output":"synthetic-result"}]}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"},{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"` + arguments + `","name":"shell_command"},"id":"` + wireID + `","type":"function"}]},{"content":"synthetic-result","role":"tool","tool_call_id":"` + wireID + `"}],"model":"gemini-3.1-pro-preview","stream":false}`
	return fixture, true
}
