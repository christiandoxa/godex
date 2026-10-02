package kiro

import (
	"encoding/json"
)

const kiroACPOutcomeField = "outcome"

func unsupportedACPServerRequest(id json.RawMessage) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": map[string]any{"code": -32601, "message": "Unsupported ACP server request"}}
}

func permissionACPServerResponse(id, params json.RawMessage) map[string]any {
	option := acpPermissionOption(params)
	outcome := map[string]any{kiroACPOutcomeField: "cancelled"}
	if option != "" {
		outcome = map[string]any{kiroACPOutcomeField: "selected", "optionId": option}
	}
	return map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{kiroACPOutcomeField: outcome}}
}

func acpPermissionOption(params json.RawMessage) string {
	var value struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if json.Unmarshal(params, &value) != nil {
		return ""
	}
	for _, kind := range []string{"allow_once", "allow_always"} {
		for _, option := range value.Options {
			if option.Kind == kind {
				return option.OptionID
			}
		}
	}
	return ""
}
