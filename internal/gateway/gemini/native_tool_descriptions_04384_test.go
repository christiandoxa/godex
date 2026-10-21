package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func geminiDescriptionTool04384(name string, schema any, description ...any) map[string]any {
	function := map[string]any{"name": name, "parameters": schema}
	if len(description) > 0 {
		function["description"] = description[0]
	}
	return map[string]any{"type": "function", "function": function}
}
func geminiNativeDeclarationSet04384(t *testing.T, model string, entries ...map[string]any) []any {
	t.Helper()
	tools := make([]any, len(entries))
	for i, item := range entries {
		tools[i] = item
	}
	chat := map[string]any{"model": model, "tools": tools}
	result, err := geminiGenerateContentRequest(chat, map[string]any{"model": model})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := result["tools"].([]any)
	if !ok || len(types) != 1 {
		t.Fatalf("Gemini %s tool cardinality=%#v", model, result["tools"])
	}
	declarations, ok := types[0].(map[string]any)["functionDeclarations"].([]any)
	if !ok {
		t.Fatalf("Gemini %s missing functionDeclarations=%#v", model, types[0])
	}
	return declarations
}
func geminiTestDeclaration04384(t *testing.T, item any) map[string]any {
	t.Helper()
	object, ok := item.(map[string]any)
	if !ok {
		t.Fatalf("invalid declaration=%#v", item)
	}
	return object
}

func TestProdex04384Gemini3ToolDescriptionsMatchMojoAliases(t *testing.T) {
	for _, tc := range []struct {
		name  string
		match string
		alias string
	}{
		{"read-file", "read_file", "Read a file with targeted, surgical ranges."},
		{"read", "read", "Read a file with targeted, surgical ranges."},
		{"qualified-read-file", " Name.Read-File ", "Read a file with targeted, surgical ranges."},
		{"mcp-suffix-read-file", "mcp__repo__read_file", "Read a file with targeted, surgical ranges."},
		{"glob", "glob", "Read multiple files by paths or globs;"},
		{"search", "search", "Search text fast"},
		{"rip-grep", "rip_grep", "Search text fast"},
		{"run-shell", "run_shell_command", "Run a shell command."},
		{"mcp-exec", "mcp__repo__exec_command", "Run a shell command."},
		{"write", "write", "Write complete file content."},
		{"apply-patch", "apply_patch", "Apply targeted file edits"},
		{"edit", "edit", "Apply targeted file edits"},
		{"mcp-patch", "mcp__repo__apply_patch", "Apply targeted file edits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := map[string]any{"type": "object"}
			got := geminiNativeDeclarationSet04384(t, "gemini-3-pro", geminiDescriptionTool04384(tc.match, schema))
			item := geminiTestDeclaration04384(t, got[0])
			description, ok := item["description"].(string)
			if !ok || !strings.HasPrefix(description, tc.alias) {
				t.Fatalf("Gemini3 description %q => %#v want prefix %q", tc.match, item["description"], tc.alias)
			}
		})
	}
}

func TestProdex04384Gemini3ToolDescriptionOverridesOnlySelectedModels(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		override    bool
	}{
		{"gemini3", "gemini-3-pro", true},
		{"uppercase-gemini3", "GEMINI-3-PRO", true},
		{"AUTO", "AUTO", true},
		{"auto-gemini3", "auto-gemini-3-pro", true},
		{"gemini2", "gemini-2.5-pro", false},
		{"gemma", "gemma-4-31b", false},
		{"unicode-gemini", "gemİni-3-pro", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := geminiNativeDeclarationSet04384(t, tc.model, geminiDescriptionTool04384("grep", map[string]any{"type": "object"}, "original text"))
			item := geminiTestDeclaration04384(t, got[0])
			want := "original text"
			if tc.override {
				want = "Search text fast with ripgrep-style behavior. Use this before broad reads when locating symbols or literals."
			}
			if item["description"] != want {
				t.Fatalf("model=%q tool description=%#v want=%q", tc.model, item["description"], want)
			}
		})
	}
}

func TestProdex04384Gemini3ToolParameterDescriptionPolicy(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"start_line": map[string]any{"type": "integer"},
		"end_line":   map[string]any{"type": "integer", "description": nil},
		"path":       map[string]any{"type": "string", "description": "keep path"},
	}}
	got := geminiNativeDeclarationSet04384(t, "gemini-3-pro", geminiDescriptionTool04384("read_file", schema))
	item := geminiTestDeclaration04384(t, got[0])
	params := item["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	if props["start_line"].(map[string]any)["description"] != "1-based first line to read when a targeted range is enough." {
		t.Fatalf("read_file start_line description missing: %#v", props)
	}
	if _, exists := props["end_line"].(map[string]any)["description"]; !exists || props["end_line"].(map[string]any)["description"] != nil {
		t.Fatalf("existing end_line description overwritten: %#v", props)
	}
	if props["path"].(map[string]any)["description"] != "keep path" {
		t.Fatalf("existing path description changed: %#v", props)
	}
}

func TestProdex04384Gemini3PatchToolAddsRequiredInputDescription(t *testing.T) {
	got := geminiNativeDeclarationSet04384(t, "gemini-3-pro", geminiDescriptionTool04384("apply_patch", map[string]any{"type": "object", "required": []any{"input"}}))
	item := geminiTestDeclaration04384(t, got[0])
	params := item["parameters"].(map[string]any)
	input := params["properties"].(map[string]any)["input"].(map[string]any)
	if input["description"] != "Full apply_patch grammar string. For Add File, prefix every new file content line with '+'." || !strings.Contains(item["description"].(string), "*** Begin Patch") {
		t.Fatalf("Gemini3 patch declarations not enriched: %#v", item)
	}
}

func TestProdex04384Gemini3ToolDescriptionRespectsNamespaceShortAliases(t *testing.T) {
	for _, name := range []string{
		"mcp__repo__read", "mcp__repo__glob", "mcp__repo__search",
		"mcp__repo__bash", "mcp__repo__write", "unknown", "réad_file", "read_file\x00",
	} {
		t.Run(name, func(t *testing.T) {
			got := geminiNativeDeclarationSet04384(t, "gemini-3-pro", geminiDescriptionTool04384(name, map[string]any{"type": "object"}, "keep"))
			if item := geminiTestDeclaration04384(t, got[0]); item["description"] != "keep" {
				t.Fatalf("qualified short alias gained a forged description: %#v", item)
			}
		})
	}
}

func TestProdex04384Gemini3PatchReplacesOnlyInputDescription(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"input"}, "properties": map[string]any{
		"input": map[string]any{"description": "replace me", "type": "string", "extra": 42},
		"other": map[string]any{"description": "keep", "type": "boolean"},
	}}
	originalInput := schema["properties"].(map[string]any)["input"].(map[string]any)
	got := geminiNativeDeclarationSet04384(t, "gemini-3-pro", geminiDescriptionTool04384("apply_patch", schema, "old tool"))
	item := geminiTestDeclaration04384(t, got[0])
	params := item["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	input := props["input"].(map[string]any)
	if input["description"] != geminiDescriptionPatchInput04384 || input["type"] != "string" || input["extra"] != 42 ||
		props["other"].(map[string]any)["description"] != "keep" || params["type"] != "object" ||
		len(params["required"].([]any)) != 1 {
		t.Fatalf("patch generated wrong schema override: %#v", item)
	}
	if originalInput["description"] != "replace me" {
		t.Fatal("tool schema input unexpectedly mutated")
	}
}

func TestProdex04384Gemini3DescriptionReachedFullResponsesTranslation(t *testing.T) {
	for _, tc := range []struct {
		model    string
		enriched bool
	}{
		{"gemini-3.1-pro-preview", true}, {"gemini-2.5-pro", false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			body := `{"model":"` + tc.model + `","input":"read a synthetic file","tools":[{` +
				`"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{` +
				`"start_line":{"type":"integer"},"path":{"type":"string"}}}}}]}`
			result, err := translateResponsesRequest([]byte(body), tc.model)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(result.body, &document); err != nil {
				t.Fatal(err)
			}
			tools := document["tools"].([]any)
			declarations := tools[0].(map[string]any)["functionDeclarations"].([]any)
			item := declarations[0].(map[string]any)
			description, _ := item["description"].(string)
			if (description == geminiDescriptionReadFile04384) != tc.enriched || (!tc.enriched && description != "") {
				t.Fatalf("full Gemini Responses declaration model=%s desc=%#v", tc.model, item["description"])
			}
			props := item["parameters"].(map[string]any)["properties"].(map[string]any)
			_, hasStart := props["start_line"].(map[string]any)["description"]
			if hasStart != tc.enriched {
				t.Fatalf("full Gemini Responses schema model=%s hasStart=%t", tc.model, hasStart)
			}
		})
	}
}
