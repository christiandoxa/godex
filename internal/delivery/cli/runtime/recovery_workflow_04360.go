package runtime

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Recovery classes are restricted to structured Codex terminal errors,
// matching the explicit variant names in Prodex 0.436.1
// usage_limit_recovery/workflow.rs. Arbitrary stderr or a bare status 429
// never authorizes relaunch.

func structuredWorkflowRecoveryClass04360(record map[string]any) string {
	kind, _ := record["type"].(string)
	var failure map[string]any
	switch {
	case kind == "event_msg":
		payload, _ := record["payload"].(map[string]any)
		if payload["type"] != "error" {
			return ""
		}
		failure = payload
	case kind == "error" || kind == "turn.failed" || kind == "turn_failed":
		failure, _ = record["error"].(map[string]any)
		if failure == nil {
			failure = record
		}
	case kind == "turn.completed" || kind == "turn_completed" || kind == "task_complete":
		turn, _ := record["turn"].(map[string]any)
		if turn == nil {
			turn = record
		}
		if turn["status"] != "failed" {
			return ""
		}
		failure, _ = turn["error"].(map[string]any)
	case record["method"] == "turn/completed":
		params, _ := record["params"].(map[string]any)
		turn, _ := params["turn"].(map[string]any)
		if turn == nil || turn["status"] != "failed" {
			return ""
		}
		failure, _ = turn["error"].(map[string]any)
	}
	if failure == nil {
		return ""
	}
	for _, scope := range []map[string]any{failure, nestedError04360(failure)} {
		if scope == nil {
			continue
		}
		if code, _ := scope["code"].(string); code == "usage_limit_reached" || code == "usage_limit_exceeded" {
			return "usage_limit"
		}
		raw, ok := scope["codex_error_info"]
		if !ok {
			raw, ok = scope["codexErrorInfo"]
		}
		if !ok {
			continue
		}
		status, hasStatus := workflowHTTPStatus04360(record, scope, raw)
		variant, _ := raw.(string)
		if names, ok := raw.(map[string]any); ok {
			// A Codex error union has exactly one active variant.
			// Ambiguous multi-key payloads must not gain replay authority.
			if len(names) != 1 {
				return ""
			}
			for key := range names {
				variant = key
			}
		}
		switch variant {
		case "usage_limit_exceeded", "usageLimitExceeded":
			return "usage_limit"
		case "rate_limit_exceeded", "rateLimitExceeded":
			return "rate_limit"
		case "server_overloaded", "serverOverloaded":
			return "overload"
		case "unauthorized":
			return "auth"
		case "internal_server_error", "internalServerError",
			"http_connection_failed", "httpConnectionFailed",
			"response_stream_connection_failed", "responseStreamConnectionFailed",
			"response_stream_disconnected", "responseStreamDisconnected",
			"response_too_many_failed_attempts", "responseTooManyFailedAttempts":
			if !hasStatus || transientWorkflowStatus04360(status) {
				return "transport"
			}
		case "other":
			return classifyUnexpectedWorkflowStatus04360(scope)
		}
	}
	return ""
}

func workflowHTTPStatus04360(record, failure map[string]any, raw any) (int, bool) {
	for _, value := range []map[string]any{record, failure} {
		if status, ok := directWorkflowHTTPStatus04360(value); ok {
			return status, true
		}
	}
	if variants, ok := raw.(map[string]any); ok {
		for _, variant := range variants {
			if value, ok := variant.(map[string]any); ok {
				if status, ok := directWorkflowHTTPStatus04360(value); ok {
					return status, true
				}
			}
		}
	}
	return 0, false
}

func directWorkflowHTTPStatus04360(value map[string]any) (int, bool) {
	for _, key := range []string{"http_status_code", "httpStatusCode", "status_code", "statusCode", "status"} {
		raw, ok := value[key]
		if !ok {
			continue
		}
		number, ok := raw.(float64)
		if ok && number >= 0 && number <= 65535 && number == float64(int(number)) {
			return int(number), true
		}
	}
	return 0, false
}

func transientWorkflowStatus04360(status int) bool {
	return status == httpStatusUnauthorized04360 || status == 429 ||
		status == 500 || status == 502 || status == 503 || status == 504 || status == 529
}

const httpStatusUnauthorized04360 = 401

func classifyUnexpectedWorkflowStatus04360(failure map[string]any) string {
	message, _ := failure["message"].(string)
	message = strings.TrimPrefix(message, "Error running remote compact task: ")
	rest, ok := strings.CutPrefix(message, "unexpected status ")
	if !ok {
		return ""
	}
	statusText, detail, ok := strings.Cut(rest, ": ")
	if !ok {
		return ""
	}
	fields := strings.Fields(statusText)
	if len(fields) == 0 {
		return ""
	}
	status, err := strconv.Atoi(fields[0])
	if err != nil {
		return ""
	}
	switch status {
	case httpStatusUnauthorized04360:
		return "auth"
	case 500, 502, 503, 504, 529:
		return "transport"
	case 402, 403:
		if authoritativeUsageLimit04360(detail) {
			return "usage_limit"
		}
		var body any
		if json.NewDecoder(strings.NewReader(detail)).Decode(&body) != nil {
			return ""
		}
		if workflowContainsCode04360(body, "deactivated_workspace") {
			return "profile_unavailable"
		}
		if workflowQuotaCode04360(body) {
			return "usage_limit"
		}
	}
	return ""
}

func authoritativeUsageLimit04360(detail string) bool {
	detail = strings.ToLower(detail)
	return strings.Contains(detail, "you've hit your usage limit") ||
		strings.Contains(detail, "you have hit your usage limit") ||
		strings.Contains(detail, "you hit your usage limit")
}

func workflowQuotaCode04360(value any) bool {
	for _, code := range []string{
		"insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded",
		"project_spend_limit_exceeded", "quota_exhausted", "quota_exceeded", "resource_exhausted",
		"usage_limit_reached", "usage_limit_exceeded", "usage_not_included", "workspace_member_credits_depleted",
	} {
		if workflowContainsCode04360(value, code) {
			return true
		}
	}
	return false
}

func workflowContainsCode04360(value any, wanted string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if (key == "code" || key == "type" || key == "status" || key == "reason") &&
				strings.EqualFold(strings.TrimSpace(stringValue04360(child)), wanted) {
				return true
			}
			if workflowContainsCode04360(child, wanted) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if workflowContainsCode04360(child, wanted) {
				return true
			}
		}
	}
	return false
}

func stringValue04360(value any) string {
	text, _ := value.(string)
	return text
}
