package main

func deepSeekToolStrictSuffixes04384() []string {
	return []string{"tool-strict-absent", "tool-strict-false-flat", "tool-strict-false-nested", "tool-strict-false-normal", "tool-strict-true", "tool-strict-false-duplicate", "tool-strict-false-absent-conflict"}
}

func deepSeekToolStrictFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	flag, wireFlag, user := `,"strict":false`, `,"strict":false`, ""
	reject := false
	switch suffix {
	case "tool-strict-absent":
		flag, wireFlag = "", ""
	case "tool-strict-false-flat", "tool-strict-false-nested", "tool-strict-false-duplicate":
	case "tool-strict-false-normal":
		user = `,"user_id":"synthetic_user"`
	case "tool-strict-true":
		flag, reject = `,"strict":true`, true
	case "tool-strict-false-absent-conflict":
		reject = true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	function := `"name":"lookup","parameters":{"type":"object"}` + flag
	tool := `{"type":"function",` + function + `}`
	if suffix == "tool-strict-false-nested" {
		tool = `{"type":"function","function":{` + function + `}}`
	}
	if suffix == "tool-strict-false-duplicate" {
		tool += `,` + tool
	}
	if suffix == "tool-strict-false-absent-conflict" {
		tool += `,{"type":"function","name":"lookup","parameters":{"type":"object"}}`
	}
	fields := ""
	if user != "" {
		fields = `,"safety_identifier":"synthetic_user"`
	}
	return requestPrimitiveFixture04384{
		body:     `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false,"tools":[` + tool + `]` + fields + `}`,
		wireBody: `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false,"tools":[{"function":{"name":"lookup","parameters":{"type":"object"}` + wireFlag + `},"type":"function"}]` + user + `}`,
		reject:   reject,
	}, true
}
