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
	// Tagged Rust reads params/options as serde_json::Value. It skips
	// unrelated scalars/objects with invalid types instead of rejecting
	// the entire options list through typed []struct unmarshalling.
	var value map[string]any
	if json.Unmarshal(params, &value) != nil {
		return ""
	}
	options, ok := value["options"].([]any)
	if !ok {
		return ""
	}
	for _, kind := range []string{"allow_once", "allow_always"} {
		for _, raw := range options {
			option, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if option["kind"] == kind {
				// Do not skip a selected allow_once with missing/non-string
				// optionId to borrow a later option. Rust selects the first
				// matching kind then requires its id to be a string.
				id, _ := option["optionId"].(string)
				return id
			}
		}
	}
	return ""
}
