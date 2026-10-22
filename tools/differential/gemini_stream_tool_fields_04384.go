package main

import (
	"io"
	"slices"
	"strings"
)

type geminiToolFieldsFixture04384 struct {
	raw, next, initialID, finalID, initialName, finalName, namespace, signature string
	index                                                                       uint64
	fallback                                                                    bool
}

func geminiToolFieldsCases04384() []string {
	return []string{"stream-chat-tool-id-absent", "stream-chat-tool-id-null", "stream-chat-tool-id-number", "stream-chat-tool-id-empty", "stream-chat-tool-id-later-empty", "stream-chat-tool-id-later-null", "stream-chat-tool-name-padded", "stream-chat-tool-name-dotted", "stream-chat-tool-name-namespace", "stream-chat-tool-name-trailing", "stream-chat-tool-name-mcp-empty", "stream-chat-tool-name-padded-search", "stream-chat-tool-name-later", "stream-chat-tool-signature", "stream-chat-tool-signature-blank", "stream-chat-tool-signature-camel", "stream-chat-tool-signature-later-blank", "stream-chat-tool-index-max", "stream-chat-tool-index-float", "stream-chat-tool-index-overflow"}
}

func geminiToolFieldsCase04384(scenario string) bool {
	return slices.Contains(geminiToolFieldsCases04384(), scenario)
}

func geminiToolFieldsFixtureFor04384(scenario string) geminiToolFieldsFixture04384 {
	f := geminiToolFieldsFixture04384{raw: `{"index":2,"id":"call_synthetic","function":{"name":"lookup","arguments":"{}"}}`, initialID: "call_synthetic", finalID: "call_synthetic", initialName: "lookup", finalName: "lookup", index: 2}
	name := ""
	switch scenario {
	case "stream-chat-tool-id-absent", "stream-chat-tool-id-null", "stream-chat-tool-id-number":
		f.fallback = true
		f.initialID = "call_deepseek_00000000-0000-7000-8000-000000000001"
		f.finalID = f.initialID
		value := ""
		if scenario == "stream-chat-tool-id-null" {
			value = `"id":null,`
		}
		if scenario == "stream-chat-tool-id-number" {
			value = `"id":7,`
		}
		f.raw = strings.Replace(f.raw, `"id":"call_synthetic",`, value, 1)
	case "stream-chat-tool-id-empty":
		f.raw = strings.Replace(f.raw, `"id":"call_synthetic"`, `"id":""`, 1)
		f.initialID, f.finalID = "", ""
	case "stream-chat-tool-id-later-empty":
		f.next, f.finalID = `{"index":2,"id":""}`, ""
	case "stream-chat-tool-id-later-null":
		f.next = `{"index":2,"id":null}`
	case "stream-chat-tool-name-padded":
		name = " lookup <>&/🌍 "
	case "stream-chat-tool-name-dotted":
		name = "synthetic.lookup"
	case "stream-chat-tool-name-namespace":
		name, f.namespace = "synthetic--lookup", "synthetic"
	case "stream-chat-tool-name-trailing":
		name = "synthetic--lookup--"
	case "stream-chat-tool-name-mcp-empty":
		name, f.namespace = "mcp____lookup", "mcp__"
	case "stream-chat-tool-name-padded-search":
		name = " tool_search "
	case "stream-chat-tool-name-later":
		f.next, f.finalName = `{"index":2,"function":{"name":"updated_lookup"}}`, "updated_lookup"
	case "stream-chat-tool-signature", "stream-chat-tool-signature-later-blank":
		f.signature = " \tsynthetic 🌍\n "
		f.raw = strings.TrimSuffix(f.raw, "}") + `,"extra_content":{"google":{"thought_signature":" \tsynthetic 🌍\n "}}}`
		if scenario == "stream-chat-tool-signature-later-blank" {
			f.next = `{"index":2,"extra_content":{"google":{"thought_signature":" \t\n "}}}`
		}
	case "stream-chat-tool-signature-blank":
		f.raw = strings.TrimSuffix(f.raw, "}") + `,"extra_content":{"google":{"thought_signature":" \t\n "}}}`
	case "stream-chat-tool-signature-camel":
		f.raw = strings.TrimSuffix(f.raw, "}") + `,"extra_content":{"google":{"thoughtSignature":"must-ignore"}}}`
	case "stream-chat-tool-index-max":
		f.raw = strings.Replace(f.raw, `"index":2`, `"index":18446744073709551615`, 1)
		f.index = ^uint64(0)
	case "stream-chat-tool-index-float", "stream-chat-tool-index-overflow":
		value := "2.0"
		if scenario == "stream-chat-tool-index-overflow" {
			value = "18446744073709551616"
		}
		f.raw, f.index = strings.Replace(f.raw, `"index":2`, `"index":`+value, 1), 0
	}
	if name != "" {
		f.raw = strings.Replace(f.raw, `"name":"lookup"`, `"name":"`+name+`"`, 1)
		f.initialName, f.finalName = name, name
		if f.namespace != "" {
			f.initialName, f.finalName = "lookup", "lookup"
		}
	}
	return f
}

func writeGeminiToolFields04384(writer io.Writer, scenario string) {
	f := geminiToolFieldsFixtureFor04384(scenario)
	_, _ = io.WriteString(writer, `data: {"created":1,"model":"gemini-3.1-pro-preview","choices":[{"delta":{"tool_calls":[`+f.raw+`]}}]}`+"\n\n")
	if f.next != "" {
		_, _ = io.WriteString(writer, `data: {"choices":[{"delta":{"tool_calls":[`+f.next+`]}}]}`+"\n\n")
	}
	_, _ = io.WriteString(writer, "data: "+`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
	_, _ = io.WriteString(writer, "data: "+`{"choices":[],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":1,"total_tokens":9007199254740994}}`+"\n\ndata: [DONE]\n\n")
}
