package main

import "strings"

func geminiMCPWireSuffixes04384() []string {
	return []string{"mcp-wire-control", "mcp-wire-tool-id", "mcp-wire-id", "mcp-wire-null-id", "mcp-wire-canonical-arguments",
		"mcp-wire-missing-arguments", "mcp-wire-null-arguments", "mcp-wire-nested-arguments", "mcp-wire-input-arguments", "mcp-wire-nested-name"}
}

func geminiMCPWireFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	fixture, known := geminiFunctionAliasFixture04384("function-alias-" + strings.TrimPrefix(suffix, "mcp-wire-"))
	fixture.body = strings.Replace(fixture.body, `"type":"function_call"`, `"type":"mcp_call"`, 1)
	return fixture, known
}
