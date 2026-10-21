package gemini

import (
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	"github.com/google/uuid"
)

type geminiStreamToolCall struct {
	index     int
	explicit  bool
	name      string
	id        string
	signature string
	item      map[string]any
	bytes     int
}

func (state *geminiStreamState) observeTool(writer io.Writer, partIndex int, part, call map[string]any) error {
	name, hasName := call["name"].(string)
	if !hasName {
		name = "tool_call"
	}
	canonical := strings.TrimSpace(name)
	if dot := strings.LastIndexByte(canonical, '.'); dot >= 0 {
		canonical = canonical[dot+1:]
	}
	if strings.ReplaceAll(geminiASCIILower04384(canonical), "-", "_") == "run_shell_command" {
		name = "exec_command"
	}
	id := nativeNonblank(call["id"])
	position, index := state.toolPosition(partIndex, id, name)
	current := geminiStreamToolCall{index: index}
	if position < len(state.toolCalls) && state.toolCalls[position].index == index {
		current = state.toolCalls[position]
	}
	if current.id == "" && id == "" {
		generated, err := uuid.NewV7()
		if err != nil {
			return errors.New("generate Gemini stream tool identity")
		}
		current.id = "call_gemini_" + generated.String()
	}
	if id != "" {
		current.id, current.explicit = id, true
	}
	current.name = name
	return state.updateTool(writer, position, current, part, call)
}

func (state *geminiStreamState) updateTool(writer io.Writer, position int, current geminiStreamToolCall, part, call map[string]any) error {
	args, exists := call["args"]
	if !exists {
		args = map[string]any{}
	}
	encoded, err := jsonutil.MarshalSerdeValue04384(args)
	if err != nil {
		return err
	}
	signature := geminiNativeThoughtSignature(part, call)
	if signature != "" {
		current.signature = signature
	}
	current.item = geminiNativeToolCallItem(nil, map[string]any{
		"name": current.name, "id": current.id, "args": args, "thoughtSignature": current.signature,
	}, current.id)
	bytes := len(encoded) + len(current.name) + len(current.id) + len(current.signature) + 128
	if input, ok := current.item["input"].(string); ok {
		bytes += len(input)
	}
	if state.toolBytes-current.bytes+bytes > bodyMaxBytes {
		return errors.New("Gemini stream tool state exceeded the safe read limit")
	}
	state.toolBytes += bytes - current.bytes
	current.bytes = bytes
	if position == len(state.toolCalls) || state.toolCalls[position].index != current.index {
		state.toolCalls = append(state.toolCalls, geminiStreamToolCall{})
		copy(state.toolCalls[position+1:], state.toolCalls[position:])
	}
	state.toolCalls[position] = current
	if current.name == "tool_search" || current.name == "apply_patch" {
		return nil
	}
	delta := map[string]any{
		"type": "response.function_call_arguments.delta", "call_id": current.id,
		"delta": string(encoded), "sequence_number": state.nextSequence(),
	}
	if signature != "" {
		delta["thought_signature"] = signature
	}
	return writeGeminiSSEEvent(writer, "response.function_call_arguments.delta", delta)
}

func (state *geminiStreamState) toolPosition(partIndex int, id, name string) (int, int) {
	// Tagged Mojo prefers explicit ownership, then anonymous position/name.
	for position, current := range state.toolCalls {
		if id != "" && current.explicit && current.id == id {
			return position, current.index
		}
		if id == "" && !current.explicit && current.index == partIndex && current.name == name {
			return position, current.index
		}
	}
	if id == "" {
		for position, current := range state.toolCalls {
			if !current.explicit && current.name == name {
				return position, current.index
			}
		}
	}
	position := sort.Search(len(state.toolCalls), func(i int) bool { return state.toolCalls[i].index >= partIndex })
	if position == len(state.toolCalls) || state.toolCalls[position].index != partIndex {
		return position, partIndex
	}
	for index := len(state.toolCalls); ; index++ {
		position := sort.Search(len(state.toolCalls), func(i int) bool { return state.toolCalls[i].index >= index })
		if position == len(state.toolCalls) || state.toolCalls[position].index != index {
			return position, index
		}
	}
}
