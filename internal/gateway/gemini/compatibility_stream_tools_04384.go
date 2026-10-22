package gemini

import (
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
)

type geminiChatStreamTool04384 struct {
	id, name, signature string
	arguments           strings.Builder
	named, added, done  bool
	bytes               int
}

func (state *geminiChatCompatibleStream043711) observeToolDelta(writer io.Writer, call map[string]any) error {
	index := nativeUint(call["index"])
	if uint64(uint(index)) != index {
		index = 0
	}
	tool := state.toolCalls[index]
	function, object := call["function"].(map[string]any)
	if tool == nil {
		if !object {
			return state.failure(writer, "invalid_tool_call_arguments", "Gemini OpenAI-compatible streamed a tool call without a function object")
		}
		id, err := uuid.NewV7()
		if err != nil {
			return errors.New("generate Gemini stream tool identity")
		}
		tool = &geminiChatStreamTool04384{id: "call_deepseek_" + id.String()}
	}
	return state.applyToolDelta(writer, index, tool, call, function)
}

func (state *geminiChatCompatibleStream043711) applyToolDelta(writer io.Writer, index uint64, tool *geminiChatStreamTool04384, call, function map[string]any) error {
	id, name, signature, named := tool.id, tool.name, tool.signature, tool.named
	if value, present := call["id"].(string); present {
		id = value
	}
	if value, present := function["name"].(string); present {
		name, named = value, true
	}
	extra, _ := call["extra_content"].(map[string]any)
	google, _ := extra["google"].(map[string]any)
	if value, _ := google["thought_signature"].(string); strings.TrimSpace(value) != "" {
		signature = value
	}
	incoming, _ := function["arguments"].(string)
	bytes := 128 + len(id) + len(name) + len(signature) + tool.arguments.Len() + len(incoming)
	// shortcut: aggregate tool retention is capped at 64 MiB; qualify larger streams before raising it.
	if bytes-tool.bytes > geminiStreamMetadataMaxBytes04384-state.toolBytes {
		state.eof = true
		return state.failure(writer, "provider_stream_error", "provider tool state exceeded safe size limit (67108864)")
	}
	state.toolBytes += bytes - tool.bytes
	tool.id, tool.name, tool.signature, tool.named, tool.bytes = id, name, signature, named, bytes
	tool.arguments.WriteString(incoming)
	if state.toolCalls == nil {
		state.toolCalls = make(map[uint64]*geminiChatStreamTool04384)
	}
	state.toolCalls[index] = tool
	return state.emitToolDelta(writer, index, tool, incoming)
}

func (state *geminiChatCompatibleStream043711) emitToolDelta(writer io.Writer, index uint64, tool *geminiChatStreamTool04384, incoming string) error {
	name := tool.name
	if !tool.named {
		name = "tool_call"
	}
	if !tool.added {
		tool.added = true
		if item := geminiChatAddedToolItem04384(tool.id, name); item != nil {
			if err := state.send(writer, "response.output_item.added", map[string]any{"output_index": index, "item": item}); err != nil {
				return err
			}
		}
	}
	if incoming == "" || name == "tool_search" || name == "apply_patch" {
		return nil
	}
	return state.send(writer, "response.function_call_arguments.delta", map[string]any{
		"output_index": index, "call_id": tool.id, "delta": incoming,
	})
}
