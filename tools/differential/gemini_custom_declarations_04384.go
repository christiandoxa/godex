package main

import (
	"encoding/json"
	"strings"
)

const geminiCustomDeclarationGuidance04384 = "Call this custom/freeform tool with the exact raw tool input in the `input` string field."
const geminiPatchDeclarationGuidance04384 = "Call this custom/freeform tool with the exact Codex apply_patch grammar in the `input` string field. The first line must be `*** Begin Patch`, the last line must be `*** End Patch`, and hunks must use `*** Add File:`, `*** Delete File:`, or `*** Update File:`. For `*** Add File: path`, every new file content line must start with `+`; for example `+hello`, and a blank content line is `+`. Do not pass unified diff headers such as `--- a/...` or `+++ b/...` as the top-level input."

func geminiCustomDeclarationSuffixes04384() []string {
	return []string{"custom-declaration-basic", "custom-declaration-patch", "custom-declaration-patch-case", "custom-declaration-description", "custom-declaration-description-blank", "custom-declaration-description-null", "custom-declaration-strict-false", "custom-declaration-strict-true", "custom-declaration-format-empty", "custom-declaration-format-values", "custom-declaration-format-null", "custom-declaration-format-type-number", "custom-declaration-name-missing", "custom-declaration-name-empty", "custom-declaration-name-invalid", "custom-declaration-duplicate", "custom-declaration-choice", "custom-declaration-stream"}
}

func geminiCustomDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture := requestPrimitiveFixture04384{}
	name, fields, description, format := "synthetic_custom", "", "Freeform custom tool input.", ""
	switch suffix {
	case "custom-declaration-basic", "custom-declaration-choice", "custom-declaration-stream", "custom-declaration-duplicate":
		fixture.geminiChatStream = suffix == "custom-declaration-stream"
	case "custom-declaration-patch":
		name = "apply_patch"
	case "custom-declaration-patch-case":
		name = "Apply_Patch"
	case "custom-declaration-description":
		fields, description = `,"description":" synthetic description "`, " synthetic description "
	case "custom-declaration-description-blank":
		fields = `,"description":" \t\u2003"`
	case "custom-declaration-description-null":
		fields, fixture.reject = `,"description":null`, true
	case "custom-declaration-strict-false":
		fields = `,"strict":false`
	case "custom-declaration-strict-true":
		fields, fixture.reject = `,"strict":true`, true
	case "custom-declaration-format-empty":
		fields, format = `,"format":{}`, "{}"
	case "custom-declaration-format-values":
		fields = `,"format":{"type":"grammar","number":1e3,"decimal":1.0,"text":"<&/🌍\u2028\u2029","u64":18446744073709551615}`
		format = `{"decimal":1.0,"number":1000.0,"text":"<&/🌍  ","type":"grammar","u64":18446744073709551615}`
	case "custom-declaration-format-null":
		fields, fixture.reject = `,"format":null`, true
	case "custom-declaration-format-type-number":
		fields, fixture.reject = `,"format":{"type":7}`, true
	case "custom-declaration-name-missing", "custom-declaration-name-empty":
		name, fixture.reject = "", true
		fixture.geminiValidationError = "invalid_tool_declaration: Gemini request field `tools[0]` is not a supported tool declaration"
	case "custom-declaration-name-invalid":
		name, fixture.reject = "invalid.name", true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	input := `{"type":"custom","name":"` + name + `"` + fields + `}`
	if suffix == "custom-declaration-name-missing" {
		input = `{"type":"custom"}`
	}
	if suffix == "custom-declaration-duplicate" {
		input += "," + input
	}
	guidance, inputDescription := geminiCustomDeclarationGuidance04384, "Exact raw input for the custom/freeform tool."
	if name == "apply_patch" {
		guidance, inputDescription = geminiPatchDeclarationGuidance04384, "Exact raw apply_patch input. For Add File, prefix every new file content line with '+'."
	}
	description += "\n\n" + guidance
	if format != "" {
		description += "\n\nOriginal custom tool format JSON: " + format
	}
	quote := func(value string) string {
		encoded, _ := json.Marshal(value)
		return strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, " ", `\u2029`, " ").Replace(string(encoded))
	}
	declaration := `{"function":{"description":` + quote(description) + `,"name":"` + name + `","parameters":{"additionalProperties":false,"properties":{"input":{"description":` + quote(inputDescription) + `,"type":"string"}},"required":["input"],"type":"object"}},"type":"function"}`
	stream, choice := "false", ""
	if fixture.geminiChatStream {
		stream = "true"
	}
	if suffix == "custom-declaration-choice" {
		choice = `,"tool_choice":{"function":{"name":"` + name + `"},"type":"function"}`
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + choice + `,"tools":[` + input + `]}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += choice + `,"tools":[` + declaration + `]}`
	return fixture, true
}
