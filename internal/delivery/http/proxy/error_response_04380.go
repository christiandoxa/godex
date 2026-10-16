package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
)

func writeProxyError(writer http.ResponseWriter, path string, status int, message string) {
	// The exact Prodex 0.438.0 Gemini bridge uses a structured
	// invalid_request_error for a locally rejected function tool. Preserve
	// its indexed message and reject it without inventing another provider
	// attempt. Unrelated 400 errors and non-Responses routes retain their
	// original text response contract.
	code := ""
	if isResponsesPath(path) {
		if status == http.StatusServiceUnavailable {
			code = "service_unavailable"
		}
		if status == http.StatusBadRequest && strings.HasPrefix(message, "invalid_tool_declaration: Gemini request field `tools") {
			code = "invalid_request_error"
		}
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
