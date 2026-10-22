package main

import (
	"fmt"
	"strings"
)

func geminiFunctionDeclarationSuffixes04384() []string {
	return []string{"declaration-empty", "declaration-flat", "declaration-nested-control", "declaration-description-empty",
		"declaration-description-blank", "declaration-description-null", "declaration-extra-outer", "declaration-extra-function",
		"declaration-strict-false", "declaration-strict-true", "declaration-strict-string", "declaration-strict-null",
		"declaration-name-invalid", "declaration-name-limit", "declaration-name-over", "declaration-duplicate-identical",
		"declaration-duplicate-numbers", "declaration-duplicate-zero", "declaration-duplicate-string-zero", "declaration-duplicate-integer-zero",
		"declaration-duplicate-conflict", "declaration-generic-first", "declaration-generic-last", "declaration-generic-conflict",
		"declaration-count-limit", "declaration-count-over", "declaration-count-duplicates", "declaration-choice", "declaration-stream"}
}

func geminiFunctionDeclarationFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const base = `{"function":{"name":"synthetic_lookup","parameters":{"type":"object"}},"type":"function"}`
	fixture := requestPrimitiveFixture04384{}
	input, wire, choice := "["+base+"]", "["+base+"]", ""
	withSchema := func(schema string) string {
		return `{"function":{"name":"synthetic_lookup","parameters":` + schema + `},"type":"function"}`
	}
	switch suffix {
	case "declaration-empty":
		input, wire = "[]", ""
	case "declaration-flat", "declaration-choice", "declaration-stream":
		input = `[{"type":"function","name":"synthetic_lookup","parameters":{"type":"object"}}]`
		fixture.geminiChatStream = suffix == "declaration-stream"
		if suffix == "declaration-choice" {
			choice = `,"tool_choice":{"function":{"name":"synthetic_lookup"},"type":"function"}`
		}
	case "declaration-nested-control":
	case "declaration-description-empty", "declaration-description-blank", "declaration-description-null":
		value := `""`
		if suffix == "declaration-description-blank" {
			value = `" \t\u2003"`
		} else if suffix == "declaration-description-null" {
			value = "null"
			fixture.reject = true
		}
		input = "[" + strings.Replace(base, `"name":`, `"description":`+value+`,"name":`, 1) + "]"
	case "declaration-extra-outer":
		input = "[" + strings.Replace(base, `"type":"function"`, `"type":"function","extra":{"value":1.0}`, 1) + "]"
	case "declaration-extra-function":
		input = "[" + strings.Replace(base, `"name":`, `"extra":{"value":1.0},"name":`, 1) + "]"
	case "declaration-strict-false", "declaration-strict-true", "declaration-strict-string", "declaration-strict-null":
		value := map[string]string{"declaration-strict-false": "false", "declaration-strict-true": "true", "declaration-strict-string": `"true"`, "declaration-strict-null": "null"}[suffix]
		input = "[" + strings.Replace(base, `"parameters":{"type":"object"}`, `"parameters":{"type":"object"},"strict":`+value, 1) + "]"
		if suffix == "declaration-strict-false" {
			wire = input
		} else {
			fixture.reject = true
		}
	case "declaration-name-invalid", "declaration-name-limit", "declaration-name-over":
		name := strings.Repeat("n", 128)
		if suffix == "declaration-name-invalid" {
			name, fixture.reject = "invalid.name", true
		} else if suffix == "declaration-name-over" {
			name += "x"
			fixture.reject = true
		}
		input = "[" + strings.Replace(base, "synthetic_lookup", name, 1) + "]"
		wire = input
	case "declaration-duplicate-identical", "declaration-count-duplicates":
		count := 2
		if suffix == "declaration-count-duplicates" {
			count = 129
		}
		input = "[" + strings.TrimSuffix(strings.Repeat(base+",", count), ",") + "]"
	case "declaration-duplicate-numbers", "declaration-duplicate-zero", "declaration-duplicate-string-zero", "declaration-duplicate-integer-zero":
		left, right := "1000.0", "1e3"
		if suffix == "declaration-duplicate-zero" {
			left, right = "-0.0", "0.0"
		} else if suffix == "declaration-duplicate-string-zero" {
			left, right, fixture.reject = `"-0.0"`, `"0.0"`, true
		} else if suffix == "declaration-duplicate-integer-zero" {
			left, right, fixture.reject = "0", "0.0", true
		}
		first, second := withSchema(`{"default":`+left+`,"type":"object"}`), withSchema(`{"default":`+right+`,"type":"object"}`)
		input, wire = "["+first+","+second+"]", "["+first+"]"
	case "declaration-duplicate-conflict":
		input, fixture.reject = "["+base+","+strings.Replace(base, `"name":`, `"description":"different","name":`, 1)+"]", true
	case "declaration-generic-first", "declaration-generic-last", "declaration-generic-conflict":
		generic, specific := withSchema(`{"additionalProperties":true,"type":"object"}`), withSchema(`{"properties":{},"required":[],"type":"object"}`)
		input, wire = "["+generic+","+specific+"]", "["+specific+"]"
		if suffix == "declaration-generic-last" {
			input = "[" + specific + "," + generic + "]"
		} else if suffix == "declaration-generic-conflict" {
			input, wire = "["+generic+","+strings.Replace(generic, `"name":`, `"description":"different","name":`, 1)+"]", "["+generic+"]"
		}
	case "declaration-count-limit", "declaration-count-over":
		count := 128
		if suffix == "declaration-count-over" {
			count, fixture.reject = 129, true
		}
		var tools []string
		for index := range count {
			tools = append(tools, strings.Replace(base, "synthetic_lookup", fmt.Sprintf("synthetic_%03d", index), 1))
		}
		input, wire = "["+strings.Join(tools, ",")+"]", "["+strings.Join(tools, ",")+"]"
	default:
		return requestPrimitiveFixture04384{}, false
	}
	stream := "false"
	if fixture.geminiChatStream {
		stream = "true"
	}
	fixture.body = `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + stream + choice + `,"tools":` + input + `}`
	fixture.wireBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream
	if fixture.geminiChatStream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	fixture.wireBody += choice
	if wire != "" {
		fixture.wireBody += `,"tools":` + wire
	}
	fixture.wireBody += "}"
	return fixture, true
}
