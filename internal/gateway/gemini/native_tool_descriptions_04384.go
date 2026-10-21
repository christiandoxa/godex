package gemini

import "strings"

// Prodex 0.438.4 gemini_tool_descriptions.mojo replaces descriptions for
// Gemini-3/auto tools and adds targeted parameter guidance. This policy is
// applied after converting OpenAI tools to Gemini functionDeclarations.
const (
	geminiDescriptionReadFile04384   = "Read a file with targeted, surgical ranges. Prefer start_line and end_line when only part of the file is needed."
	geminiDescriptionReadMany04384   = "Read multiple files by paths or globs; honor git, Gemini, custom ignore rules, and default heavy-directory excludes."
	geminiDescriptionGrep04384       = "Search text fast with ripgrep-style behavior. Use this before broad reads when locating symbols or literals."
	geminiDescriptionExec04384       = "Run a shell command. Prefer non-interactive commands and continue background sessions until the needed output is available."
	geminiDescriptionWrite04384      = "Write complete file content. Do not use placeholders; preserve unrelated user changes."
	geminiDescriptionPatch04384      = "Apply targeted file edits with exact context. Keep edits narrow and use this instead of describing changes. For Add File patches, every content line must start with '+', for example: *** Begin Patch\\n*** Add File: note.txt\\n+hello\\n*** End Patch."
	geminiDescriptionPatchInput04384 = "Full apply_patch grammar string. For Add File, prefix every new file content line with '+'."
	geminiDescriptionStartLine04384  = "1-based first line to read when a targeted range is enough."
	geminiDescriptionEndLine04384    = "1-based last line to read, inclusive."
	geminiDescriptionPattern04384    = "Literal or regex pattern to search for."
)

func gemini3ToolDescriptionModel04384(model string) bool {
	folded := geminiASCIILower04384(model)
	return strings.Contains(folded, "gemini-3") || folded == "auto"
}

// This models tooling_name_policy: trim Unicode space, take the final dot
// suffix, ASCII-casefold, normalize '-' to '_', then inspect the full name
// and the last double-underscore namespace suffix separately. Short aliases
// are recognized on a bare name but not under an MCP namespace.
func gemini3DescriptionKind04384(name string) int {
	name = strings.TrimSpace(name)
	if last := strings.LastIndexByte(name, '.'); last >= 0 {
		name = name[last+1:]
	}
	name = strings.ReplaceAll(geminiASCIILower04384(name), "-", "_")
	if name == "" {
		return 0
	}
	switch name {
	case "read_file", "read":
		return 1
	case "read_many_files", "glob":
		return 2
	case "grep", "rg", "rip_grep", "search":
		return 3
	case "exec_command", "run_shell_command", "shell", "bash":
		return 4
	case "write_file", "write":
		return 5
	case "apply_patch", "replace", "edit":
		return 6
	}
	if last := strings.LastIndex(name, "__"); last >= 0 {
		switch name[last+2:] {
		case "read_file":
			return 1
		case "read_many_files":
			return 2
		case "grep", "rg", "rip_grep":
			return 3
		case "exec_command", "run_shell_command":
			return 4
		case "write_file":
			return 5
		case "apply_patch", "replace", "edit":
			return 6
		}
	}
	return 0
}

func gemini3DescriptionText04384(kind int) string {
	switch kind {
	case 1:
		return geminiDescriptionReadFile04384
	case 2:
		return geminiDescriptionReadMany04384
	case 3:
		return geminiDescriptionGrep04384
	case 4:
		return geminiDescriptionExec04384
	case 5:
		return geminiDescriptionWrite04384
	case 6:
		return geminiDescriptionPatch04384
	default:
		return ""
	}
}

func gemini3DescriptionProperty04384(properties map[string]any, field, description string) {
	original, ok := properties[field].(map[string]any)
	if !ok {
		return
	}
	if _, exists := original["description"]; exists {
		return
	}
	cloned := make(map[string]any, len(original)+1)
	for key, value := range original {
		cloned[key] = value
	}
	cloned["description"] = description
	properties[field] = cloned
}

func gemini3ApplyDescriptions04384(declarations []any, model string) {
	if !gemini3ToolDescriptionModel04384(model) {
		return
	}
	for _, raw := range declarations {
		decl, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := decl["name"].(string)
		kind := gemini3DescriptionKind04384(name)
		if kind == 0 {
			continue
		}
		decl["description"] = gemini3DescriptionText04384(kind)
		parameters, ok := decl["parameters"].(map[string]any)
		if !ok {
			if kind != 6 {
				continue
			}
			parameters = make(map[string]any)
		}
		// Copy the path being modified: original request schema and
		// unrelated properties must remain untouched by the translator.
		copied := make(map[string]any, len(parameters)+1)
		for key, value := range parameters {
			copied[key] = value
		}
		properties, ok := parameters["properties"].(map[string]any)
		if !ok {
			if kind != 6 {
				continue
			}
			properties = make(map[string]any)
		}
		clonedProperties := make(map[string]any, len(properties)+1)
		for key, value := range properties {
			clonedProperties[key] = value
		}
		switch kind {
		case 1:
			gemini3DescriptionProperty04384(clonedProperties, "start_line", geminiDescriptionStartLine04384)
			gemini3DescriptionProperty04384(clonedProperties, "end_line", geminiDescriptionEndLine04384)
		case 3:
			gemini3DescriptionProperty04384(clonedProperties, "pattern", geminiDescriptionPattern04384)
		case 6:
			input, _ := clonedProperties["input"].(map[string]any)
			clonedInput := make(map[string]any, len(input)+1)
			for key, value := range input {
				if key != "description" {
					clonedInput[key] = value
				}
			}
			clonedInput["description"] = geminiDescriptionPatchInput04384
			clonedProperties["input"] = clonedInput
		default:
			continue
		}
		copied["properties"] = clonedProperties
		decl["parameters"] = copied
	}
}
