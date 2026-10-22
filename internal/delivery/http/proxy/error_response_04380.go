package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
)

func writeProxyError(writer http.ResponseWriter, path string, status int, message string) {
	// Tagged Gemini local tool/candidate failures use a structured Responses error.
	code := ""
	if status == http.StatusGatewayTimeout && message == requestRecoveryTimeoutMessage {
		code = "request_timeout"
	}
	if isResponsesPath(path) {
		if status == http.StatusServiceUnavailable {
			code = "service_unavailable"
		}
		if status == http.StatusBadRequest {
			switch message {
			case "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1",
				"invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1",
				"invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict":
				code = "invalid_request_error"
			}
			if strings.HasPrefix(message, "invalid_tool_declaration: Gemini request field `tools") {
				code = "invalid_request_error"
			}
		}
	}
	if status == http.StatusServiceUnavailable && message == "bound continuation provider is unavailable" {
		code = "bound_continuation_unavailable"
	}
	if code != "" {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(status)
		payload, _ := json.Marshal(struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}{Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message}})
		_, _ = writer.Write(payload)
		return
	}
	writeTextResponse(writer, status, message)
}

func isResponsesPath(path string) bool {
	path = strings.TrimRight(path, "/")
	return strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact")
}
