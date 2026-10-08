package proxy

import (
	"net/http"
	"strings"
)

// These are the optional Codex bootstrap paths handled locally by the exact
// Prodex 0.436.0 standard-lane admission pressure policy. The response is a
// safe empty catalog/acknowledgment, never a synthesized model response.
func startupMetadataResponse(path string) (int, string, bool) {
	switch normalizedStartupPath(path) {
	case "/backend-api/codex/analytics-events/events":
		return http.StatusNoContent, "", true
	case "/backend-api/plugins/featured",
		"/backend-api/ps/plugins/installed",
		"/backend-api/connectors/directory/list":
		return http.StatusOK, `{"items":[],"data":[]}`, true
	default:
		return 0, "", false
	}
}

func startupStandardPriorityPath(path string) bool {
	switch normalizedStartupPath(path) {
	case "/backend-api/codex/models", "/backend-api/ps/mcp":
		return true
	default:
		return false
	}
}

// Normalize the same backend-api aliases recognized by the OpenAI transport.
// Keep this policy in HTTP admission; routing and providers do not create
// synthetic catalog results as part of their ordinary upstream behavior.
func normalizedStartupPath(path string) string {
	path, _, _ = strings.Cut(path, "?")
	for _, mount := range []string{"/backend-api/prodex", "/backend-api/godex"} {
		suffix, ok := strings.CutPrefix(path, mount)
		if !ok || suffix != "" && !strings.HasPrefix(suffix, "/") {
			continue
		}
		if strings.HasPrefix(suffix, "/v") {
			version, remaining, hasTail := strings.Cut(suffix[2:], "/")
			if hasTail && startupVersionSegment(version) {
				suffix = "/" + remaining
			}
		}
		return "/backend-api/codex" + suffix
	}
	return path
}

func startupVersionSegment(value string) bool {
	if value == "" {
		return false
	}
	sawDigit := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			sawDigit = true
			continue
		}
		if r != '.' {
			return false
		}
	}
	return sawDigit
}

func (handler *activeRequestHandler) shedOptionalStartupMetadata(writer http.ResponseWriter, path string) bool {
	status, body, optional := startupMetadataResponse(path)
	if !optional {
		return false
	}
	handler.mu.Lock()
	active, limit := handler.laneActive[admissionLaneStandard], handler.limits.lane[admissionLaneStandard]
	handler.mu.Unlock()
	if active < max(1, max(limit, 1)/2) {
		return false
	}
	if status == http.StatusOK {
		writer.Header().Set("Content-Type", "application/json")
	}
	writer.WriteHeader(status)
	if body != "" {
		_, _ = writer.Write([]byte(body))
	}
	return true
}
