package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func (state *geminiChatCompatibleStream043711) orderedToolIndices() []uint64 {
	indices := make([]uint64, 0, len(state.toolCalls))
	for index := range state.toolCalls {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	return indices
}

func (state *geminiChatCompatibleStream043711) completeToolEvents(writer io.Writer) (bool, error) {
	indices := state.orderedToolIndices()
	for _, index := range indices {
		if err := validateGeminiChatStreamTool04384(index, state.toolCalls[index]); err != nil {
			return false, state.failure(writer, "invalid_tool_call_arguments", err.Error())
		}
	}
	for _, index := range indices {
		tool := state.toolCalls[index]
		if tool.done {
			continue
		}
		arguments := chatcompat.WrapRTKArguments(tool.name, tool.arguments.String())
		if len(arguments)-tool.arguments.Len() > geminiStreamMetadataMaxBytes04384-state.toolBytes {
			state.eof = true
			return false, state.failure(writer, "provider_stream_error", "provider tool state exceeded safe size limit (67108864)")
		}
		if arguments != tool.arguments.String() {
			state.toolBytes += len(arguments) - tool.arguments.Len()
			tool.bytes += len(arguments) - tool.arguments.Len()
			tool.arguments.Reset()
			tool.arguments.WriteString(arguments)
		}
		tool.done = true
		if err := state.send(writer, "response.output_item.done", map[string]any{"output_index": index, "item": geminiChatCompletedToolItem04384(tool)}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func validateGeminiChatStreamTool04384(index uint64, tool *geminiChatStreamTool04384) error {
	name := strings.TrimSpace(tool.name)
	if name == "" {
		return fmt.Errorf("Gemini OpenAI-compatible streamed a tool call without a function name at index %d", index)
	}
	arguments := tool.arguments.String()
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	if jsonutil.CheckSerdeBoundary(arguments) != nil || !json.Valid([]byte(arguments)) || !jsonutil.CheckSerdeNumberTokens([]byte(arguments)) {
		return fmt.Errorf("Gemini OpenAI-compatible streamed malformed JSON arguments for tool call `%s` at index %d: invalid JSON", name, index)
	}
	return nil
}
