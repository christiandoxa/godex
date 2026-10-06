package superexpose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func canonicalExposeTool(name string) string {
	switch name {
	case godexStartToolName, legacyStartToolName:
		return godexStartToolName
	case godexStatusToolName, legacyStatusToolName:
		return godexStatusToolName
	case godexEventsToolName, legacyEventsToolName:
		return godexEventsToolName
	case godexResultToolName, legacyResultToolName:
		return godexResultToolName
	case godexCancelToolName, legacyCancelToolName:
		return godexCancelToolName
	case godexListToolName, legacyListToolName:
		return godexListToolName
	case godexExecToolName, legacyExecToolName:
		return godexExecToolName
	case godexSessionPromptWriteToolName, legacySessionPromptWriteToolName:
		return godexSessionPromptWriteToolName
	case godexSessionPreemptToolName, legacySessionPreemptToolName:
		return godexSessionPreemptToolName
	case godexSessionOutputReadToolName, legacySessionOutputReadToolName:
		return godexSessionOutputReadToolName
	default:
		return name
	}
}

func (handler *execMCPHandler) callTool(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	tool := canonicalExposeTool(name)
	if handler.mode != "full" && tool != godexExecToolName {
		return nil, errors.New("tool is not exposed by this endpoint")
	}
	if sessionTool(tool) {
		return handler.callSessionTool(tool, arguments)
	}
	switch tool {
	case godexExecToolName:
		return executeDirectAudited(ctx, arguments, handler.workspace, handler.optionalTools, handler.audit)
	case godexStartToolName:
		if handler.runs == nil {
			return nil, errors.New("run manager is unavailable")
		}
		handler.runs.setAuditIfNil(handler.audit)
		started, err := handler.runs.start(arguments)
		if err != nil {
			return nil, err
		}
		return map[string]any{"run_id": started["run_id"], "state": started["state"]}, nil
	case godexStatusToolName:
		runID, err := requiredRunID(arguments)
		if err != nil {
			return nil, err
		}
		value := handler.runs.status(runID)
		value["instance_id"] = handler.instanceID
		return value, nil
	case godexEventsToolName:
		runID, err := requiredRunID(arguments)
		if err != nil {
			return nil, err
		}
		after, err := optionalUint(arguments, "after_seq", 0)
		if err != nil {
			return nil, err
		}
		limit64, err := optionalUint(arguments, "limit", maxEventPage)
		if err != nil {
			return nil, err
		}
		if limit64 < 1 || limit64 > maxEventPage {
			return nil, fmt.Errorf("limit must be between 1 and %d", maxEventPage)
		}
		value := handler.runs.events(runID, after, int(limit64))
		value["instance_id"] = handler.instanceID
		return value, nil
	case godexResultToolName:
		runID, err := requiredRunID(arguments)
		if err != nil {
			return nil, err
		}
		value := handler.runs.result(runID)
		value["instance_id"] = handler.instanceID
		return value, nil
	case godexCancelToolName:
		runID, err := requiredRunID(arguments)
		if err != nil {
			return nil, err
		}
		value := handler.runs.cancel(runID)
		value["instance_id"] = handler.instanceID
		return value, nil
	case godexListToolName:
		return map[string]any{"instance_id": handler.instanceID, "runs": handler.runs.list()}, nil
	default:
		return nil, errors.New("tool is not exposed by this endpoint")
	}
}

func validateToolArguments(name string, arguments map[string]any) error {
	tool := canonicalExposeTool(name)
	if sessionTool(tool) {
		return validateSessionToolArguments(tool, arguments)
	}
	allowed := map[string]bool{}
	switch tool {
	case godexStartToolName:
		for _, key := range []string{"task", "model", "reasoning_effort", "provider", "profile", "sub_agents"} {
			allowed[key] = true
		}
	case godexStatusToolName, godexResultToolName, godexCancelToolName:
		allowed["run_id"] = true
	case godexEventsToolName:
		allowed["run_id"] = true
		allowed["after_seq"] = true
		allowed["limit"] = true
	case godexListToolName:
	case godexExecToolName:
		for _, key := range []string{"program", "args", "cwd", "env", "stdin", "timeout_ms"} {
			allowed[key] = true
		}
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

func requiredRunID(arguments map[string]any) (string, error) {
	raw, ok := arguments["run_id"]
	if !ok {
		return "", errors.New("run_id is required")
	}
	value, ok := raw.(string)
	if !ok || value == "" || len(value) > 128 || !validRunID(value) {
		return "", errors.New("run_id is invalid")
	}
	return value, nil
}

func optionalUint(arguments map[string]any, name string, fallback uint64) (uint64, error) {
	raw, ok := arguments[name]
	if !ok || raw == nil {
		return fallback, nil
	}
	switch value := raw.(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil || parsed < 0 {
			return 0, errors.New("value must be a non-negative integer")
		}
		return uint64(parsed), nil
	case float64:
		if value < 0 || value != float64(uint64(value)) {
			return 0, errors.New("value must be a non-negative integer")
		}
		return uint64(value), nil
	default:
		return 0, errors.New("value must be a non-negative integer")
	}
}

func (handler *execMCPHandler) lifecycleToolDefinitions() []any {
	runIDSchema := map[string]any{"type": "object", "properties": map[string]any{"run_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}, "required": []string{"run_id"}, "additionalProperties": false}
	statusSchema := map[string]any{"type": "object", "properties": map[string]any{"instance_id": map[string]any{"type": "string"}, "run_id": map[string]any{"type": "string"}, "state": map[string]any{"type": "string"}, "created_at": map[string]any{"type": "integer"}, "started_at": map[string]any{"type": []string{"integer", "null"}}, "finished_at": map[string]any{"type": []string{"integer", "null"}}, "exit_status": map[string]any{"type": []string{"integer", "null"}}, "provider": map[string]any{"type": []string{"string", "null"}}, "model": map[string]any{"type": []string{"string", "null"}}, "reasoning_effort": map[string]any{"type": []string{"string", "null"}}, "cancellation_requested": map[string]any{"type": "boolean"}}, "required": []string{"instance_id", "run_id", "state"}}
	return []any{
		exposeToolDefinition(godexStartToolName, "Start one full-access Godex Super task in the captured initial working directory. Poll its run_id instead of starting duplicates.", map[string]any{"type": "object", "properties": map[string]any{"task": map[string]any{"type": "string", "minLength": 1, "maxLength": 65536}, "model": map[string]any{"type": []string{"string", "null"}, "maxLength": 256}, "reasoning_effort": map[string]any{"type": []string{"string", "null"}, "maxLength": 256}, "provider": map[string]any{"type": []string{"string", "null"}, "maxLength": 256}, "profile": map[string]any{"type": []string{"string", "null"}, "maxLength": 128}, "sub_agents": map[string]any{"type": []string{"boolean", "null"}}}, "required": []string{"task"}, "additionalProperties": false}, map[string]any{"type": "object", "properties": map[string]any{"run_id": map[string]any{"type": "string"}, "state": map[string]any{"type": "string"}}, "required": []string{"run_id", "state"}}, false, true, true),
		exposeToolDefinition(godexStatusToolName, "Read the current bounded state of one Godex Super run.", runIDSchema, statusSchema, true, false, false),
		exposeToolDefinition(godexEventsToolName, "Read a bounded monotonic page of redacted stdout/stderr lifecycle events for one run.", map[string]any{"type": "object", "properties": map[string]any{"run_id": map[string]any{"type": "string"}, "after_seq": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxEventPage}}, "required": []string{"run_id"}, "additionalProperties": false}, map[string]any{"type": "object"}, true, false, false),
		exposeToolDefinition(godexResultToolName, "Read a bounded final result for one Godex Super run, or its current nonterminal state.", runIDSchema, map[string]any{"type": "object"}, true, false, false),
		exposeToolDefinition(godexCancelToolName, "Cancel one Godex Super run and terminate only its child process tree.", runIDSchema, statusSchema, false, true, false),
		exposeToolDefinition(godexListToolName, "List bounded runs owned by this expose instance.", map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, map[string]any{"type": "object"}, true, false, false),
	}
}

func exposeToolDefinition(name, description string, input, output map[string]any, readOnly, destructive, openWorld bool) map[string]any {
	return map[string]any{"name": name, "title": name, "description": description, "inputSchema": input, "outputSchema": output, "annotations": map[string]any{"readOnlyHint": readOnly, "destructiveHint": destructive, "openWorldHint": openWorld}}
}
