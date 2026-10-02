package kiro

import (
	"fmt"
	"os"
	"strings"
)

type runtimeRequest struct {
	model              string
	effort             string
	prompt             string
	stream             bool
	previousResponseID string
	toolOutputCallID   string
	callIDs            []string
}

func parseKiroResponsesRequest(body []byte, allowTokenLimit bool) (runtimeRequest, error) {
	return parseKiroResponsesRequestForAgent(body, allowTokenLimit, kiroSubAgent())
}

func parseKiroResponsesRequestForAgent(body []byte, allowTokenLimit, subAgent bool) (runtimeRequest, error) {
	object, err := decodeRuntimeObject(body, "Responses")
	if err != nil {
		return runtimeRequest{}, err
	}
	stripKiroExternalToolControls(object, subAgent)
	if err := validateKiroResponsesControls(object, allowTokenLimit); err != nil {
		return runtimeRequest{}, err
	}
	prompt, err := kiroResponsesPrompt(object)
	if err != nil {
		return runtimeRequest{}, err
	}
	return runtimeRequest{
		model: runtimeString(object[kiroFieldModel], "auto"), effort: runtimeReasoningEffort(object),
		prompt: prompt, stream: runtimeBool(object[kiroFieldStream]),
		previousResponseID: runtimeString(object["previous_response_id"], ""),
		toolOutputCallID:   firstKiroToolOutputCallID(object[kiroFieldInput]),
		callIDs:            kiroFunctionCallIDs(object[kiroFieldInput]),
	}, nil
}

func parseKiroChatRequest(body []byte) (runtimeRequest, error) {
	return parseKiroChatRequestForAgent(body, kiroSubAgent())
}

func parseKiroChatRequestForAgent(body []byte, subAgent bool) (runtimeRequest, error) {
	object, err := decodeRuntimeObject(body, "chat completions")
	if err != nil {
		return runtimeRequest{}, err
	}
	stripKiroExternalToolControls(object, subAgent)
	normalizeLegacyKiroFunctionCall(object)
	if err := validateKiroChatControls(object); err != nil {
		return runtimeRequest{}, err
	}
	messages, ok := object["messages"].([]any)
	if !ok || len(messages) == 0 {
		return runtimeRequest{}, newKiroRequestError("missing_messages", "Kiro chat completions request is missing messages")
	}
	prompt := promptFromMessages(nil, messages)
	if strings.TrimSpace(prompt) == "" {
		return runtimeRequest{}, newKiroRequestError("invalid_messages", "Kiro chat completions request contains no text messages")
	}
	return runtimeRequest{
		model:  runtimeString(object[kiroFieldModel], "auto"),
		prompt: prompt,
		stream: runtimeBool(object[kiroFieldStream]),
	}, nil
}

func parseKiroMessagesRequest(body []byte) (runtimeRequest, error) {
	return parseKiroMessagesRequestForAgent(body, kiroSubAgent())
}

func parseKiroMessagesRequestForAgent(body []byte, subAgent bool) (runtimeRequest, error) {
	object, err := decodeRuntimeObject(body, "Responses")
	if err != nil {
		return runtimeRequest{}, err
	}
	stripKiroExternalToolControls(object, subAgent)
	if err := validateKiroResponsesControls(object, true); err != nil {
		return runtimeRequest{}, err
	}
	messages, ok := object["messages"].([]any)
	if !ok || len(messages) == 0 {
		return runtimeRequest{}, newKiroRequestError("missing_messages", "Kiro Messages request is missing messages")
	}
	var system []any
	switch value := object[kiroRoleSystem].(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			system = []any{map[string]any{"role": kiroRoleSystem, kiroFieldContent: value}}
		}
	case []any:
		text, err := runtimeTextContent(value)
		if err != nil {
			return runtimeRequest{}, err
		}
		if text != "" {
			system = []any{map[string]any{"role": kiroRoleSystem, kiroFieldContent: text}}
		}
	}
	prompt := promptFromMessages(system, messages)
	if strings.TrimSpace(prompt) == "" {
		return runtimeRequest{}, newKiroRequestError("invalid_messages", "Kiro Messages request contains no text messages")
	}
	return runtimeRequest{
		model:  runtimeString(object[kiroFieldModel], "auto"),
		prompt: prompt,
		stream: runtimeBool(object[kiroFieldStream]),
	}, nil
}

func kiroSubAgent() bool {
	return os.Getenv("PRODEX_SUB_AGENT") != ""
}

func stripKiroExternalToolControls(object map[string]any, subAgent bool) {
	fields := []string{"tools", "functions"}
	if subAgent {
		fields = []string{
			"parallel_tool_calls", "tool_choice", "tools",
			"functions", "function_call", "web_search_options",
		}
	}
	for _, field := range fields {
		delete(object, field)
	}
}

func kiroResponsesPrompt(object map[string]any) (string, error) {
	var prefix []any
	if instructions, ok := object["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
		prefix = append(prefix, map[string]any{"role": kiroRoleSystem, kiroFieldContent: instructions})
	}
	input, found := object[kiroFieldInput]
	if !found {
		return "", newKiroRequestError(kiroErrorInvalidRequest, "Kiro Responses request is missing input")
	}
	switch value := input.(type) {
	case string:
		return promptFromMessages(prefix, []any{map[string]any{"role": "user", kiroFieldContent: value}}), nil
	case []any:
		return promptFromResponseItems(prefix, value)
	default:
		return "", newKiroRequestError(kiroErrorInvalidRequest, "Kiro Responses input must be text or an item array")
	}
}

func promptFromResponseItems(prefix, items []any) (string, error) {
	messages := append([]any(nil), prefix...)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		converted, err := kiroResponseItemMessages(item)
		if err != nil {
			return "", err
		}
		messages = append(messages, converted...)
	}
	prompt := promptFromMessages(nil, messages)
	if strings.TrimSpace(prompt) == "" {
		return "", newKiroRequestError(kiroErrorInvalidRequest, "Kiro Responses request contains no text input")
	}
	return prompt, nil
}

func kiroResponseItemMessages(item map[string]any) ([]any, error) {
	kind, _ := item["type"].(string)
	switch kind {
	case "message":
		content, err := runtimeTextContent(item[kiroFieldContent])
		if err != nil {
			return nil, err
		}
		return []any{map[string]any{"role": runtimeString(item["role"], "user"), kiroFieldContent: content}}, nil
	case "function_call":
		return []any{map[string]any{"role": "assistant", kiroFieldContent: "Tool call " + runtimeString(item["name"], "tool") + ": " + runtimeCompact(item["arguments"])}}, nil
	case "function_call_output":
		return []any{map[string]any{"role": "tool", kiroFieldContent: runtimeCompact(item["output"])}}, nil
	case "reasoning":
		return kiroReasoningMessage(item)
	case "input_text", "output_text":
		if text, ok := item["text"].(string); ok && text != "" {
			return []any{map[string]any{"role": "user", kiroFieldContent: text}}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("Kiro ACP does not support Responses item type %q", kind)
	}
}

func kiroReasoningMessage(item map[string]any) ([]any, error) {
	summary, err := runtimeTextContent(item["summary"])
	if err != nil || summary == "" {
		return nil, err
	}
	return []any{map[string]any{"role": "assistant", kiroFieldContent: summary}}, nil
}

func promptFromMessages(prefix, messages []any) string {
	all := append(append([]any(nil), prefix...), messages...)
	sections := make([]string, 0, len(all))
	for _, raw := range all {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		section := kiroChatPromptSection(message)
		if section == "" {
			continue
		}
		sections = append(sections, section)
	}
	if len(sections) == 0 {
		return "User:\n"
	}
	return strings.Join(sections, "\n\n")
}
