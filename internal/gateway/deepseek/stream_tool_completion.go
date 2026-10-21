package deepseek

func (state *deepSeekChatStreamState) validateToolCalls() error {
	for index, call := range state.toolCalls {
		if err := deepSeekValidateStreamToolCall(index, call); err != nil {
			return err
		}
	}
	return nil
}

func (state *deepSeekChatStreamState) completeToolEvents(items []deepSeekCompletedStreamTool) [][]byte {
	var events [][]byte
	for _, tool := range items {
		call := state.toolCalls[tool.index]
		if call.done {
			continue
		}
		call.done = true
		events = append(events, anthropicStreamEvent("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "sequence_number": state.nextSequenceNumber(),
			"output_index": tool.index, "item": tool.item,
		}))
	}
	return events
}
