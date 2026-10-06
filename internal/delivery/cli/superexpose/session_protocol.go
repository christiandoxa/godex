package superexpose

import (
	"encoding/json"
	"errors"
	"fmt"
)

func sessionTool(name string) bool {
	switch name {
	case godexSessionPromptWriteToolName, godexSessionPreemptToolName, godexSessionOutputReadToolName:
		return true
	default:
		return false
	}
}

func (handler *execMCPHandler) callSessionTool(name string, arguments map[string]any) (map[string]any, error) {
	if handler.sessions == nil {
		return nil, errors.New("existing-session control is unavailable")
	}
	pid, err := sessionOptionalPID(arguments)
	if err != nil {
		return nil, err
	}
	threadID, err := sessionOptionalThreadID(arguments)
	if err != nil {
		return nil, err
	}
	bindingKey := sessionBindingKey(handler.instanceID, pid, threadID)
	switch name {
	case godexSessionPromptWriteToolName:
		message, err := requiredSessionString(arguments, "message", sessionPromptMaxMessageBytes)
		if err != nil {
			return nil, err
		}
		cwd, err := optionalSessionString(arguments, "cwd", 4096)
		if err != nil {
			return nil, err
		}
		return handler.sessions.write(sessionPromptWriteRequest{
			workspaceRoot: handler.workspace, message: message, cwd: cwd,
			godexPID: pid, threadID: threadID, bindingKey: bindingKey,
		})
	case godexSessionPreemptToolName:
		cwd, err := optionalSessionString(arguments, "cwd", 4096)
		if err != nil {
			return nil, err
		}
		return handler.sessions.preempt(sessionPreemptRequest{
			workspaceRoot: handler.workspace, cwd: cwd,
			godexPID: pid, threadID: threadID, bindingKey: bindingKey,
		})
	case godexSessionOutputReadToolName:
		cursor, err := optionalSessionString(arguments, "cursor", outputCursorMaxBytes)
		if err != nil {
			return nil, err
		}
		limit, err := sessionOptionalBoundedUint(arguments, "limit", 100, 1, 200)
		if err != nil {
			return nil, err
		}
		waitMS, err := sessionOptionalBoundedUint(arguments, "wait_ms", 0, 0, 10_000)
		if err != nil {
			return nil, err
		}
		return handler.sessions.readOutput(outputReadRequest{
			workspaceRoot: handler.workspace, cursor: cursor,
			limit: int(limit), waitMS: waitMS,
			godexPID: pid, threadID: threadID, bindingKey: bindingKey,
		})
	default:
		return nil, errors.New("tool is not exposed by this endpoint")
	}
}

func validateSessionToolArguments(name string, arguments map[string]any) error {
	allowed := map[string]bool{"godex_pid": true, "prodex_pid": true, "thread_id": true}
	switch name {
	case godexSessionPromptWriteToolName:
		allowed["message"] = true
		allowed["cwd"] = true
	case godexSessionPreemptToolName:
		allowed["cwd"] = true
	case godexSessionOutputReadToolName:
		allowed["cursor"] = true
		allowed["limit"] = true
		allowed["wait_ms"] = true
	default:
		return nil
	}
	for key := range arguments {
		if !allowed[key] {
			return fmt.Errorf("unknown tool argument: %s", key)
		}
	}
	return nil
}

func requiredSessionString(arguments map[string]any, name string, maxBytes int) (string, error) {
	raw, ok := arguments[name]
	if !ok {
		return "", fmt.Errorf("%s is required", name)
	}
	value, ok := raw.(string)
	if !ok || value == "" || len(value) > maxBytes {
		return "", fmt.Errorf("%s is empty or too large", name)
	}
	return value, nil
}

func optionalSessionString(arguments map[string]any, name string, maxBytes int) (string, error) {
	raw, exists := arguments[name]
	if !exists || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	if value == "" || len(value) > maxBytes {
		return "", fmt.Errorf("%s is empty or too large", name)
	}
	return value, nil
}

func sessionOptionalPID(arguments map[string]any) (*uint32, error) {
	raw, exists := arguments["godex_pid"]
	if !exists {
		raw, exists = arguments["prodex_pid"]
	}
	if !exists || raw == nil {
		return nil, nil
	}
	value, ok := sessionUint(raw)
	if !ok || value == 0 || value > uint64(^uint32(0)) {
		return nil, errors.New("godex_pid is invalid")
	}
	result := uint32(value)
	return &result, nil
}

func sessionOptionalThreadID(arguments map[string]any) (string, error) {
	value, err := optionalSessionString(arguments, "thread_id", 128)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", nil
	}
	value, err = normalizedThreadID(value)
	if err != nil {
		return "", errors.New("thread_id is invalid")
	}
	return value, nil
}

func sessionOptionalBoundedUint(arguments map[string]any, name string, fallback, minimum, maximum uint64) (uint64, error) {
	raw, exists := arguments[name]
	if !exists || raw == nil {
		return fallback, nil
	}
	value, ok := sessionUint(raw)
	if !ok || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func sessionUint(raw any) (uint64, bool) {
	switch value := raw.(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil || parsed < 0 {
			return 0, false
		}
		return uint64(parsed), true
	case float64:
		if value < 0 || value != float64(uint64(value)) {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}

func (handler *execMCPHandler) sessionToolDefinitions() []any {
	sessionIdentityProperties := map[string]any{
		"godex_pid": map[string]any{"type": []string{"integer", "null"}, "minimum": 1},
		"thread_id": map[string]any{"type": []string{"string", "null"}, "maxLength": 128},
	}
	promptProperties := cloneSchemaProperties(sessionIdentityProperties)
	promptProperties["message"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 65536}
	promptProperties["cwd"] = map[string]any{"type": []string{"string", "null"}, "maxLength": 4096}
	preemptProperties := cloneSchemaProperties(sessionIdentityProperties)
	preemptProperties["cwd"] = map[string]any{"type": []string{"string", "null"}, "maxLength": 4096}
	readProperties := cloneSchemaProperties(sessionIdentityProperties)
	readProperties["cursor"] = map[string]any{"type": []string{"string", "null"}, "maxLength": 16384}
	readProperties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 200}
	readProperties["wait_ms"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}

	return []any{
		exposeToolDefinition(
			godexSessionPromptWriteToolName,
			"Prompt Write: deliver one session input to an already-running plain godex s through the supported Codex control plane. Identity checks fail closed and the tool never starts another solver.",
			map[string]any{"type": "object", "properties": promptProperties, "required": []string{"message"}, "additionalProperties": false},
			map[string]any{"type": "object"},
			false, false, false,
		),
		exposeToolDefinition(
			godexSessionPreemptToolName,
			"Preempt the current turn for one exact existing plain godex s session, then remove every still-pending queued prompt through Codex. Ambiguous state fails closed.",
			map[string]any{"type": "object", "properties": preemptProperties, "additionalProperties": false},
			map[string]any{"type": "object"},
			false, true, false,
		),
		exposeToolDefinition(
			godexSessionOutputReadToolName,
			"Read bounded user-visible output from the same already-running plain godex s interactive session. It never reads the PTY and uses identity-bound cursors.",
			map[string]any{"type": "object", "properties": readProperties, "additionalProperties": false},
			map[string]any{"type": "object"},
			true, false, false,
		),
	}
}

func cloneSchemaProperties(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+3)
	for key, value := range source {
		result[key] = value
	}
	return result
}
