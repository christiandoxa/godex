package gemini

func (state *geminiChatCompatibleStream043711) rememberConversation() {
	if state.conversation == nil {
		return
	}
	message := map[string]any{"content": state.text.String()}
	if state.reasoning.Len() > 0 {
		message["reasoning_content"] = state.reasoning.String()
		if state.text.Len() == 0 && len(state.toolCalls) == 0 {
			message["content"] = nil
		}
	}
	if len(state.toolCalls) > 0 {
		calls := make([]any, 0, len(state.toolCalls))
		for _, index := range state.orderedToolIndices() {
			tool := state.toolCalls[index]
			call := map[string]any{
				"id": tool.id, "type": "function",
				"function": map[string]any{"name": tool.name, "arguments": tool.arguments.String()},
			}
			if tool.signature != "" {
				call["extra_content"] = map[string]any{"google": map[string]any{"thought_signature": tool.signature}}
			}
			calls = append(calls, call)
		}
		message["tool_calls"] = calls
	}
	source := map[string]any{"choices": []any{map[string]any{"message": message}}}
	state.conversation.remember(map[string]any{"id": state.id}, source)
}
