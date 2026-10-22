package gemini

import "strings"

func geminiChatBuiltinTool04384(tool map[string]any) bool {
	kind, _ := tool["type"].(string)
	if geminiChatBuiltinKind04384(kind) {
		return true
	}
	for _, key := range []string{"computerUse", "codeExecution", "urlContext"} {
		if _, present := tool[key]; present {
			return true
		}
	}
	return false
}

func geminiChatBuiltinKind04384(kind string) bool {
	switch kind {
	case "web_search", "web_search_preview", "web_fetch", "url_context", "urlContext", "web_fetch_preview",
		"code_interpreter", "code_execution", "codeExecution", "computer", "computer_use", "computerUse", "computer_use_preview":
		return true
	}
	return strings.HasPrefix(kind, "web_search_preview_") || strings.HasPrefix(kind, "web_fetch_preview_") || strings.HasPrefix(kind, "computer_")
}

func geminiChatBuiltinFunction04384(tool map[string]any) (any, bool) {
	kind, typed := tool["type"].(string)
	function, _ := tool["function"].(map[string]any)
	if typed && (!geminiChatBuiltinKind04384(kind) || !validGeminiChatFunctionFields04384(tool) || !validGeminiChatFunctionFields04384(function)) {
		return nil, false
	}
	if kind == "web_search" || kind == "web_search_preview" || strings.HasPrefix(kind, "web_search_preview_") {
		return nil, true
	}
	projected := projectGeminiChatMCPAlias04384(tool)
	if len(projected) == 0 {
		return nil, true
	}
	return geminiChatFunctionDeclaration04384(projected[0].(map[string]any))
}
