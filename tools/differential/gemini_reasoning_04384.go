package main

import "strings"

func geminiReasoningSuffixes04384() []string {
	return []string{"reasoning-empty", "reasoning-low", "reasoning-medium", "reasoning-high", "reasoning-xhigh", "reasoning-max", "reasoning-minimal", "reasoning-none", "reasoning-ascii", "reasoning-unicode",
		"reasoning-nested-precedence", "reasoning-empty-alias", "reasoning-null", "reasoning-unknown-key", "reasoning-null-effort", "reasoning-number-alias", "reasoning-null-alias", "reasoning-unknown-effort", "reasoning-empty-effort", "reasoning-null-shadow",
		"reasoning-tools-high", "reasoning-tools-minimal", "reasoning-tools-none", "reasoning-tools-signature"}
}

func geminiReasoningFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	if strings.HasPrefix(suffix, "reasoning-tools-") {
		effort := strings.TrimPrefix(suffix, "reasoning-tools-")
		if effort == "signature" {
			effort = "high"
		}
		fixture, _ := geminiFunctionAliasFixture04384("function-alias-control")
		if suffix == "reasoning-tools-signature" {
			fixture.body = strings.Replace(fixture.body, `"arguments":"{}"`, `"arguments":"{}","thought_signature":"synthetic-signature"`, 1)
			fixture.wireBody = strings.Replace(fixture.wireBody, `"tool_calls":[{"function"`, `"tool_calls":[{"extra_content":{"google":{"thought_signature":"synthetic-signature"}},"function"`, 1)
		}
		fixture.body = strings.TrimSuffix(fixture.body, "}") + `,"reasoning":{"effort":"` + effort + `"}}`
		fixture.wireBody = strings.Replace(fixture.wireBody, `,"stream":false`, `,"reasoning_effort":"`+effort+`","stream":false`, 1)
		if effort == "high" {
			fixture.wireBody = strings.Replace(fixture.wireBody, `"role":"assistant"`, `"reasoning_content":"","role":"assistant"`, 1)
		}
		return fixture, true
	}
	fields, effort := `"reasoning":{}`, ""
	fixture := requestPrimitiveFixture04384{}
	switch suffix {
	case "reasoning-empty":
	case "reasoning-low":
		fields, effort = `"reasoning":{"effort":"low"}`, "low"
	case "reasoning-medium":
		fields, effort = `"reasoning":{"effort":"medium"}`, "medium"
	case "reasoning-high":
		fields, effort = `"reasoning":{"effort":"high"}`, "high"
	case "reasoning-xhigh":
		fields, effort = `"reasoning_effort":"xhigh"`, "high"
	case "reasoning-max":
		fields, effort = `"reasoning_effort":"max"`, "high"
	case "reasoning-minimal":
		fields, effort = `"reasoning_effort":"minimal"`, "minimal"
	case "reasoning-none":
		fields, effort = `"reasoning_effort":"none"`, "none"
	case "reasoning-ascii":
		fields, effort = `"reasoning_effort":" \tHiGh\u2003"`, "high"
	case "reasoning-unicode":
		fields, fixture.reject = `"reasoning_effort":"hİgh"`, true
	case "reasoning-nested-precedence":
		fields, effort = `"reasoning":{"effort":"low"},"reasoning_effort":false`, "low"
	case "reasoning-empty-alias":
		fields, effort = `"reasoning":{},"reasoning_effort":"medium"`, "medium"
	case "reasoning-null":
		fields, fixture.reject = `"reasoning":null`, true
	case "reasoning-unknown-key":
		fields, fixture.reject = `"reasoning":{"effort":"low","summary":"auto"}`, true
	case "reasoning-null-effort":
		fields, fixture.reject = `"reasoning":{"effort":null}`, true
	case "reasoning-number-alias":
		fields, fixture.reject = `"reasoning_effort":7`, true
	case "reasoning-null-alias":
		fields, fixture.reject = `"reasoning_effort":null`, true
	case "reasoning-unknown-effort":
		fields, fixture.reject = `"reasoning_effort":"future"`, true
	case "reasoning-empty-effort":
		fields, fixture.reject = `"reasoning_effort":""`, true
	case "reasoning-null-shadow":
		fields, fixture.reject = `"reasoning":null,"reasoning_effort":"high"`, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request",` + fields + `}`
	wire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"`
	if effort != "" {
		wire += `,"reasoning_effort":"` + effort + `"`
	}
	fixture.wireBody = wire + `,"stream":false}`
	return fixture, true
}
