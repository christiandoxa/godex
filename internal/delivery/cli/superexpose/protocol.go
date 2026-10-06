package superexpose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/version"
)

const (
	mcpCurrentProtocolVersion = "2026-07-28"
	mcpMaxJSONNesting         = 64
	mcpErrorUnsupported       = -32022
	mcpErrorHeaderMismatch    = -32020
)

var mcpProtocolVersions = []string{
	mcpCurrentProtocolVersion,
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

const (
	godexStartToolName              = "godex_super_start"
	godexStatusToolName             = "godex_super_status"
	godexEventsToolName             = "godex_super_events"
	godexResultToolName             = "godex_super_result"
	godexCancelToolName             = "godex_super_cancel"
	godexListToolName               = "godex_super_list"
	godexExecToolName               = "godex_super_exec"
	godexSessionPromptWriteToolName = "godex_session_prompt_write"
	godexSessionPreemptToolName     = "godex_session_preempt"
	godexSessionOutputReadToolName  = "godex_session_output_read"

	legacyStartToolName              = "prodex_super_start"
	legacyStatusToolName             = "prodex_super_status"
	legacyEventsToolName             = "prodex_super_events"
	legacyResultToolName             = "prodex_super_result"
	legacyCancelToolName             = "prodex_super_cancel"
	legacyListToolName               = "prodex_super_list"
	legacyExecToolName               = "prodex_super_exec"
	legacySessionPromptWriteToolName = "prodex_session_prompt_write"
	legacySessionPreemptToolName     = "prodex_session_preempt"
	legacySessionOutputReadToolName  = "prodex_session_output_read"
)

type execMCPHandler struct {
	expectedPath  string
	expectedHost  string
	displayName   string
	instanceID    string
	workspace     string
	mode          string
	runs          *runManager
	sessions      *existingSessionService
	optionalTools optionalToolSnapshot
	audit         *exposeAuditLog
	rate          rateLimiter
}

func (handler *execMCPHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != handler.expectedPath {
		handler.audit.tryEvent("super_expose_http_rejected", map[string]string{"reason": "not_found"})
		writeJSON(writer, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if request.Method != http.MethodPost {
		handler.audit.tryEvent("super_expose_http_rejected", map[string]string{"reason": "method_not_allowed"})
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if !handler.rate.admit(time.Now()) {
		handler.audit.tryEvent("super_expose_http_rejected", map[string]string{"reason": "rate_limit"})
		writeRPCError(writer, http.StatusTooManyRequests, nil, -32029, "request rate limit exceeded", nil)
		return
	}
	if !mcpContentTypeAllowed(uniqueHeader(request.Header, "Content-Type")) {
		writeRPCError(writer, http.StatusUnsupportedMediaType, nil, -32600, "content type must be application/json", nil)
		return
	}
	if !mcpAcceptAllowed(uniqueHeader(request.Header, "Accept")) {
		writeRPCError(writer, http.StatusNotAcceptable, nil, -32600, "accept must include application/json", nil)
		return
	}
	origins := request.Header.Values("Origin")
	if len(origins) > 1 || !mcpOriginAllowed(request.Host, uniqueHeader(request.Header, "Origin")) {
		writeRPCError(writer, http.StatusForbidden, nil, -32003, "origin rejected", nil)
		return
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, bodyMaxBytes+1))
	if err != nil || len(body) > bodyMaxBytes {
		handler.audit.tryEvent("super_expose_http_rejected", map[string]string{
			"reason":     "invalid_body",
			"body_bytes": fmt.Sprint(len(body)),
		})
		writeRPCError(writer, http.StatusBadRequest, nil, -32600, "request body is invalid or too large", nil)
		return
	}
	if !mcpJSONNestingWithinLimit(body, mcpMaxJSONNesting) {
		writeRPCError(writer, http.StatusBadRequest, nil, -32700, "parse error", nil)
		return
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		handler.audit.tryEvent("super_expose_rpc_rejected", map[string]string{
			"mode": handler.mode, "reason": "parse_error",
		})
		writeRPCError(writer, http.StatusBadRequest, nil, -32700, "parse error", nil)
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		handler.audit.tryEvent("super_expose_rpc_rejected", map[string]string{
			"mode": handler.mode, "reason": "parse_error",
		})
		writeRPCError(writer, http.StatusBadRequest, nil, -32700, "parse error", nil)
		return
	}
	message, ok := raw.(map[string]any)
	if !ok {
		handler.audit.tryEvent("super_expose_rpc_rejected", map[string]string{
			"mode": handler.mode, "reason": "invalid_request",
		})
		detail := "parse error"
		if _, isArray := raw.([]any); isArray {
			detail = "batch requests are unsupported"
		}
		writeRPCError(writer, http.StatusBadRequest, nil, -32600, detail, nil)
		return
	}
	if message["jsonrpc"] != "2.0" {
		writeRPCError(writer, http.StatusBadRequest, requestID(message), -32600, "invalid request", nil)
		return
	}
	method, ok := message["method"].(string)
	if !ok || method == "" {
		writeRPCError(writer, http.StatusBadRequest, requestID(message), -32600, "method is required", nil)
		return
	}
	params, paramsPresent := message["params"]
	paramsObject, paramsIsObject := params.(map[string]any)
	id, hasID, validID := requestIDState(message)
	if err := validateMCPMetadata(
		method,
		paramsObject,
		request.Header,
	); err != nil {
		writeRPCError(writer, http.StatusBadRequest, requestID(message), err.code, err.message, err.data)
		return
	}
	if !hasID {
		if method == "notifications/initialized" || method == "notifications/cancelled" {
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPCError(writer, http.StatusBadRequest, nil, -32601, "notification is unsupported", nil)
		return
	}
	if !validID {
		writeRPCError(writer, http.StatusBadRequest, nil, -32600, "invalid request id", nil)
		return
	}

	toolName := ""
	if paramsObject != nil {
		toolName, _ = paramsObject["name"].(string)
	}
	methodAudit, toolAudit := exposeAuditRoute(method, toolName)
	handler.audit.tryEvent("super_expose_rpc", map[string]string{
		"mode": handler.mode, "method": methodAudit, "tool": toolAudit,
		"body_bytes": fmt.Sprint(len(body)),
	})

	switch method {
	case "server/discover":
		handler.auditRPCCompletion(methodAudit, toolAudit, true)
		writeRPCResult(writer, id, handler.serverDiscover())
	case "initialize":
		if !paramsPresent || !paramsIsObject {
			writeRPCError(writer, http.StatusBadRequest, id, -32602, "initialize params are required", nil)
			return
		}
		protocolVersion, ok := paramsObject["protocolVersion"].(string)
		if !ok || protocolVersion == "" {
			writeRPCError(writer, http.StatusBadRequest, id, -32602, "protocolVersion is required", nil)
			return
		}
		if !mcpProtocolVersionSupported(protocolVersion) {
			handler.auditRPCCompletion(methodAudit, toolAudit, false)
			writeRPCResult(writer, id, toolErrorResult("unsupported protocol version"))
			return
		}
		handler.auditRPCCompletion(methodAudit, toolAudit, true)
		writeRPCResult(writer, id, handler.initialize(protocolVersion))
	case "ping":
		handler.auditRPCCompletion(methodAudit, toolAudit, true)
		writeRPCResult(writer, id, map[string]any{})
	case "tools/list":
		handler.auditRPCCompletion(methodAudit, toolAudit, true)
		writeRPCResult(writer, id, handler.toolsList())
	case "tools/call":
		if !paramsPresent || !paramsIsObject {
			writeRPCError(writer, http.StatusBadRequest, id, -32602, "tool parameters are required", nil)
			return
		}
		name, ok := paramsObject["name"].(string)
		if !ok || name == "" {
			writeRPCError(writer, http.StatusBadRequest, id, -32602, "tool name is required", nil)
			return
		}
		arguments := map[string]any{}
		if rawArguments, exists := paramsObject["arguments"]; exists {
			var ok bool
			arguments, ok = rawArguments.(map[string]any)
			if !ok {
				writeRPCError(writer, http.StatusBadRequest, id, -32602, "tool arguments must be an object", nil)
				return
			}
		}
		if err := validateToolArguments(name, arguments); err != nil {
			writeRPCError(writer, http.StatusBadRequest, id, -32602, err.Error(), nil)
			return
		}
		result, err := handler.callTool(request.Context(), name, arguments)
		if err != nil {
			handler.auditRPCCompletion(methodAudit, toolAudit, false)
			writeRPCResult(writer, id, toolErrorResult(err.Error()))
			return
		}
		handler.auditRPCCompletion(methodAudit, toolAudit, true)
		writeRPCResult(writer, id, toolSuccessResult(result))
	default:
		handler.audit.tryEvent("super_expose_rpc_rejected", map[string]string{
			"mode": handler.mode, "reason": "method_not_found",
		})
		writeRPCError(writer, http.StatusNotFound, id, -32601, "method not found", nil)
	}
}

func (handler *execMCPHandler) auditRPCCompletion(method, tool string, success bool) {
	handler.audit.tryEvent("super_expose_rpc_completed", map[string]string{
		"mode": handler.mode, "method": method, "tool": tool, "success": fmt.Sprint(success),
	})
}

func exposeAuditRoute(method, tool string) (string, string) {
	methodLabel := "unknown"
	switch method {
	case "server/discover":
		methodLabel = "server_discover"
	case "initialize":
		methodLabel = "initialize"
	case "ping":
		methodLabel = "ping"
	case "tools/list":
		methodLabel = "tools_list"
	case "tools/call":
		methodLabel = "tools_call"
	case "notifications/initialized", "notifications/cancelled":
		methodLabel = "notification"
	}
	toolLabel := "unknown"
	if method == "tools/call" {
		switch canonicalExposeTool(tool) {
		case godexStartToolName:
			toolLabel = "start"
		case godexStatusToolName:
			toolLabel = "status"
		case godexEventsToolName:
			toolLabel = "events"
		case godexResultToolName:
			toolLabel = "result"
		case godexCancelToolName:
			toolLabel = "cancel"
		case godexListToolName:
			toolLabel = "list"
		case godexExecToolName:
			toolLabel = "exec"
		case godexSessionPromptWriteToolName:
			toolLabel = "session_prompt_write"
		case godexSessionPreemptToolName:
			toolLabel = "session_preempt"
		case godexSessionOutputReadToolName:
			toolLabel = "session_output_read"
		}
	}
	return methodLabel, toolLabel
}

func (handler *execMCPHandler) serverName() string {
	return "Godex Super — " + handler.displayName
}

func (handler *execMCPHandler) instructions() string {
	if handler.mode == "full" {
		return "Full Godex Super endpoint. Run lifecycle tools and godex_super_exec are exposed for this local capability instance.\n" + handler.optionalTools.instructions()
	}
	return "Exec-only Godex Super endpoint. Only godex_super_exec is exposed.\n" + handler.optionalTools.instructions()
}

func (handler *execMCPHandler) serverDiscover() map[string]any {
	return map[string]any{
		"resultType":        "complete",
		"supportedVersions": []string{mcpCurrentProtocolVersion},
		"capabilities":      map[string]any{"tools": map[string]any{"listChanged": false}},
		"instructions":      handler.instructions(),
		"ttlMs":             300_000,
		"cacheScope":        "private",
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": map[string]any{
				"name":    handler.serverName(),
				"version": version.Version,
			},
		},
	}
}

func (handler *execMCPHandler) initialize(protocolVersion string) map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name":    handler.serverName(),
			"version": version.Version,
		},
		"instructions": handler.instructions(),
	}
}

func (handler *execMCPHandler) toolsList() map[string]any {
	tools := []any{handler.execToolDefinition()}
	if handler.mode == "full" {
		full := handler.lifecycleToolDefinitions()
		full = append(full, handler.sessionToolDefinitions()...)
		tools = append(full, tools...)
	}
	return map[string]any{
		"resultType": "complete",
		"tools":      tools,
		"ttlMs":      300_000,
		"cacheScope": "private",
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": map[string]any{
				"name":    handler.serverName(),
				"version": version.Version,
			},
		},
	}
}

func (handler *execMCPHandler) execToolDefinition() map[string]any {
	description := "Execute one direct OS command under the expose process's current local OS-user authority. It is synchronous and standalone: it does not require, create, attach to, or depend on a Godex Super run or plain godex s session."
	available := handler.optionalTools.availableIDs()
	if len(available) == 0 {
		description += " No validated Godex optional-tool aliases are active for this endpoint."
	} else {
		aliases := make([]string, 0, len(available)+1)
		for _, id := range available {
			aliases = append(aliases, "optional:"+id)
			if id == "playwright-mcp" {
				aliases = append(aliases, "optional:playwright")
			}
		}
		description += " Validated optional-tool aliases available at endpoint start: " + strings.Join(aliases, ", ") + ". Prefer these aliases over guessing PATH locations."
	}
	return map[string]any{
		"name":        godexExecToolName,
		"title":       "godex super exec",
		"description": description,
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"program":    map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"args":       map[string]any{"type": []string{"array", "null"}, "maxItems": 256, "items": map[string]any{"type": "string", "maxLength": 16384}},
				"cwd":        map[string]any{"type": []string{"string", "null"}, "maxLength": 4096},
				"env":        map[string]any{"type": []string{"object", "null"}, "maxProperties": 128, "additionalProperties": map[string]any{"type": "string", "maxLength": 16384}, "propertyNames": map[string]any{"maxLength": 256}},
				"stdin":      map[string]any{"type": []string{"string", "null"}, "maxLength": 262144},
				"timeout_ms": map[string]any{"type": []string{"integer", "null"}, "minimum": 1, "maximum": 120000, "default": 30000},
			},
			"required":                []string{"program"},
			"additionalProperties":    false,
			"x-maxTotalArgumentBytes": 262144,
		},
		"outputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":                   map[string]any{"type": "string", "enum": []string{"completed", "timed_out", "cancelled"}},
				"program":                  map[string]any{"type": "string"},
				"optional_tool":            map[string]any{"type": []string{"string", "null"}},
				"available_optional_tools": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"arg_count":                map[string]any{"type": "integer"},
				"cwd":                      map[string]any{"type": "string"},
				"pid":                      map[string]any{"type": []string{"integer", "null"}},
				"exit_code":                map[string]any{"type": []string{"integer", "null"}},
				"exit_status":              map[string]any{"type": "integer"},
				"signal":                   map[string]any{"type": []string{"integer", "null"}},
				"termination":              map[string]any{"type": []string{"string", "null"}},
				"success":                  map[string]any{"type": "boolean"},
				"duration_ms":              map[string]any{"type": "integer"},
				"stdout":                   map[string]any{"type": "string", "maxLength": 131072},
				"stdout_truncated":         map[string]any{"type": "boolean"},
				"stderr":                   map[string]any{"type": "string", "maxLength": 131072},
				"stderr_truncated":         map[string]any{"type": "boolean"},
			},
			"required": []string{
				"status", "program", "optional_tool", "available_optional_tools", "arg_count",
				"cwd", "pid", "exit_code", "exit_status", "signal", "termination", "success",
				"duration_ms", "stdout", "stdout_truncated", "stderr", "stderr_truncated",
			},
		},
		"annotations": map[string]any{
			"readOnlyHint":    false,
			"destructiveHint": true,
			"openWorldHint":   true,
		},
		"_meta": map[string]any{
			"godex/optionalTools": handler.optionalTools.manifest(),
		},
	}
}

func toolSuccessResult(result any) map[string]any {
	return map[string]any{
		"resultType":        "complete",
		"content":           []any{map[string]any{"type": "text", "text": mustJSON(result)}},
		"structuredContent": result,
		"isError":           false,
	}
}

func toolErrorResult(message string) map[string]any {
	result := map[string]any{"error": message}
	return map[string]any{
		"resultType":        "complete",
		"content":           []any{map[string]any{"type": "text", "text": mustJSON(result)}},
		"structuredContent": result,
		"isError":           true,
	}
}

func validateExecToolArguments(name string, arguments map[string]any) error {
	if !isExecToolName(name) {
		return nil
	}
	allowed := map[string]bool{
		"program": true, "args": true, "cwd": true, "env": true, "stdin": true, "timeout_ms": true,
	}
	for key := range arguments {
		if !allowed[key] {
			return fmt.Errorf("unknown tool argument: %s", key)
		}
	}
	return nil
}

func isExecToolName(name string) bool {
	return name == godexExecToolName || name == legacyExecToolName
}

type metadataError struct {
	code    int
	message string
	data    any
}

func validateMCPMetadata(method string, params map[string]any, header http.Header) *metadataError {
	headerVersion := uniqueHeader(header, "MCP-Protocol-Version")
	bodyVersion := bodyProtocolVersion(method, params)
	if headerVersion != "" && !mcpProtocolVersionSupported(headerVersion) {
		return &metadataError{
			code: mcpErrorUnsupported, message: "unsupported protocol version",
			data: map[string]any{"supported": mcpProtocolVersions, "requested": headerVersion},
		}
	}
	if bodyVersion != "" && !mcpProtocolVersionSupported(bodyVersion) {
		return &metadataError{
			code: mcpErrorUnsupported, message: "unsupported protocol version",
			data: map[string]any{"supported": mcpProtocolVersions, "requested": bodyVersion},
		}
	}
	if headerVersion != "" && bodyVersion != "" && headerVersion != bodyVersion {
		return &metadataError{code: mcpErrorHeaderMismatch, message: "protocol version header mismatch"}
	}
	current := headerVersion == mcpCurrentProtocolVersion || bodyVersion == mcpCurrentProtocolVersion
	methodHeader := uniqueHeader(header, "Mcp-Method")
	nameHeader := uniqueHeader(header, "Mcp-Name")
	if current {
		if headerVersion != mcpCurrentProtocolVersion || bodyVersion != mcpCurrentProtocolVersion {
			return &metadataError{code: mcpErrorHeaderMismatch, message: "protocol version metadata is required"}
		}
		if methodHeader != method {
			return &metadataError{code: mcpErrorHeaderMismatch, message: "Mcp-Method header mismatch"}
		}
		if method == "tools/call" {
			bodyName, _ := params["name"].(string)
			if nameHeader != bodyName {
				return &metadataError{code: mcpErrorHeaderMismatch, message: "Mcp-Name header mismatch"}
			}
		}
	} else if methodHeader != "" && methodHeader != method {
		return &metadataError{code: mcpErrorHeaderMismatch, message: "Mcp-Method header mismatch"}
	}
	return nil
}

func bodyProtocolVersion(method string, params map[string]any) string {
	if params == nil {
		return ""
	}
	if method == "initialize" {
		value, _ := params["protocolVersion"].(string)
		return value
	}
	meta, _ := params["_meta"].(map[string]any)
	value, _ := meta["io.modelcontextprotocol/protocolVersion"].(string)
	return value
}

func requestID(message map[string]any) any {
	id, _, valid := requestIDState(message)
	if !valid {
		return nil
	}
	return id
}

func requestIDState(message map[string]any) (any, bool, bool) {
	id, exists := message["id"]
	if !exists {
		return nil, false, false
	}
	switch id.(type) {
	case string, json.Number, float64:
		return id, true, true
	default:
		return nil, true, false
	}
}

func mcpProtocolVersionSupported(value string) bool {
	for _, supported := range mcpProtocolVersions {
		if value == supported {
			return true
		}
	}
	return false
}

func mcpContentTypeAllowed(value string) bool {
	if value == "" {
		return false
	}
	mediaType := strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
	return strings.EqualFold(mediaType, "application/json")
}

func mcpAcceptAllowed(value string) bool {
	if value == "" {
		return false
	}
	for _, item := range strings.Split(value, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(strings.TrimSpace(item), ";", 2)[0])
		if strings.EqualFold(mediaType, "application/json") {
			return true
		}
	}
	return false
}

func mcpOriginAllowed(host, origin string) bool {
	if origin == "" {
		return true
	}
	if origin != strings.TrimSpace(origin) {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	localHTTP := strings.HasPrefix(host, "127.0.0.1:") && origin == "http://"+host
	originHost := parsed.Hostname()
	trustedHTTPS := parsed.Scheme == "https" &&
		(strings.EqualFold(originHost, host) ||
			strings.EqualFold(originHost, "chatgpt.com") ||
			strings.EqualFold(originHost, "chat.openai.com")) &&
		parsed.Port() == ""
	return (localHTTP || trustedHTTPS) &&
		parsed.User == nil &&
		(parsed.Path == "" || parsed.Path == "/") &&
		parsed.RawQuery == "" &&
		parsed.Fragment == ""
}

func uniqueHeader(header http.Header, name string) string {
	values := header.Values(name)
	if len(values) != 1 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func mcpJSONNestingWithinLimit(body []byte, limit int) bool {
	depth := 0
	inString := false
	escaped := false
	for _, current := range body {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == '"' {
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > limit {
				return false
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return !inString && !escaped && depth == 0
}

func writeRPCResult(writer http.ResponseWriter, id, result any) {
	writeJSON(writer, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeRPCError(writer http.ResponseWriter, status int, id any, code int, message string, data any) {
	rpcError := map[string]any{"code": code, "message": message}
	if data != nil {
		rpcError["data"] = data
	}
	writeJSON(writer, status, map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
