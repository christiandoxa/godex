package runtime

// Recovery classes are restricted to structured Codex terminal errors,
// matching the explicit variant names in Prodex 0.436.0
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
			return "transport"
		}
	}
	return ""
}
