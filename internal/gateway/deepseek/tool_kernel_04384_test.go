package deepseek

import (
	"strings"
	"testing"
)

const (
	toolKernelViewBytes04384  = 4 << 20
	toolKernelInputBytes04384 = 16 << 20
)

func TestProdex04384BufferedToolKernelMetadataBounds(t *testing.T) {
	const callID = "call-kernel"
	const bounded = "DeepSeek response tool call exceeds the bounded Mojo kernel limit"
	const searchArgs = toolKernelInputBytes04384 - toolKernelViewBytes04384 - len("tool_search") - len(callID)
	for _, fixture := range []struct {
		name, tool, arguments, want        string
		idBytes, signatureBytes, argsBytes int
		omitID                             bool
	}{
		{name: "blank_name_limit", tool: strings.Repeat(" ", toolKernelViewBytes04384), want: "DeepSeek returned a tool call without a function name"},
		{name: "blank_name_over", tool: strings.Repeat(" ", toolKernelViewBytes04384+1), want: "DeepSeek returned a tool call name exceeding the supported size"},
		{name: "call_id_limit", idBytes: toolKernelViewBytes04384},
		{name: "call_id_over", idBytes: toolKernelViewBytes04384 + 1, want: bounded},
		{name: "signature_limit", signatureBytes: toolKernelViewBytes04384},
		{name: "signature_over", signatureBytes: toolKernelViewBytes04384 + 1, want: bounded},
		{name: "aggregate_limit", tool: "tool_search", signatureBytes: toolKernelViewBytes04384, argsBytes: searchArgs},
		{name: "aggregate_over", tool: "tool_search", signatureBytes: toolKernelViewBytes04384, argsBytes: searchArgs + 1, want: bounded},
		{name: "missing_id_limit", tool: "tool_search", omitID: true, signatureBytes: toolKernelViewBytes04384, argsBytes: searchArgs + len(callID)},
		{name: "missing_id_over", tool: "tool_search", omitID: true, signatureBytes: toolKernelViewBytes04384, argsBytes: searchArgs + len(callID) + 1, want: bounded},
		{name: "earlier_number_error", signatureBytes: toolKernelViewBytes04384 + 1, arguments: "1e309", want: "DeepSeek returned malformed JSON arguments for tool call `lookup`: number out of range at line 1 column 5"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			name, arguments := fixture.tool, fixture.arguments
			if name == "" {
				name = "lookup"
			}
			if arguments == "" {
				arguments = "{}"
			}
			if fixture.argsBytes > 0 {
				arguments += strings.Repeat(" ", fixture.argsBytes-len(arguments))
			}
			id := callID
			if fixture.idBytes > 0 {
				id = strings.Repeat("i", fixture.idBytes)
			}
			call := map[string]any{"function": map[string]any{"name": name, "arguments": arguments},
				"extra_content": map[string]any{"google": map[string]any{"thought_signature": strings.Repeat(" ", fixture.signatureBytes)}}}
			if !fixture.omitID {
				call["id"] = id
			}
			item, err := deepSeekResponseToolItem(call)
			if fixture.want == "" {
				if err != nil || item == nil {
					t.Fatalf("kernel boundary control rejected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != fixture.want || item != nil {
				t.Fatalf("kernel bound rejection = %v, want %q", err, fixture.want)
			}
		})
	}
}

func TestProdex04384BufferedToolKernelCountsRTKWrappedArguments(t *testing.T) {
	const name, callID = "shell", "call-kernel"
	const prefix, suffix = `{"cmd":"rg synthetic","pad":"`, `"}`
	size := toolKernelInputBytes04384 - len(name) - len(callID)
	arguments := prefix + strings.Repeat(" ", size-len(prefix)-len(suffix)) + suffix
	_, err := deepSeekResponseToolItem(map[string]any{"id": callID,
		"function": map[string]any{"name": name, "arguments": arguments}})
	if err == nil || err.Error() != "DeepSeek response tool call exceeds the bounded Mojo kernel limit" {
		t.Fatalf("wrapped arguments bypassed aggregate admission: %v", err)
	}
}
