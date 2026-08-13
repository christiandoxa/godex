package openai

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type responseKind uint8

const (
	responsePass responseKind = iota
	responseRetry
	responseAuthFailure
)

type responseOutcome struct {
	kind       responseKind
	quarantine time.Duration
}

type pendingResponse struct {
	response  *http.Response
	prefix    []byte
	accountID string
}

func (pending *pendingResponse) close() {
	if pending != nil && pending.response != nil && pending.response.Body != nil {
		pending.response.Body.Close()
	}
}

func (proxy *Proxy) classify(response *http.Response) (responseOutcome, *pendingResponse) {
	pending := &pendingResponse{response: response}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return responseOutcome{kind: responseAuthFailure}, pending
	case response.StatusCode == http.StatusTooManyRequests:
		return responseOutcome{kind: responseRetry, quarantine: retryAfter(response.Header, proxy.now())}, pending
	case response.StatusCode == http.StatusInternalServerError ||
		response.StatusCode == http.StatusBadGateway ||
		response.StatusCode == http.StatusServiceUnavailable ||
		response.StatusCode == http.StatusGatewayTimeout:
		return responseOutcome{kind: responseRetry}, pending
	case response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusForbidden:
		prefix, complete := inspectResponse(response.Body, proxy.maxInspect)
		pending.prefix = prefix
		if complete && isQuotaResponse(prefix) {
			return responseOutcome{kind: responseRetry, quarantine: 30 * time.Second}, pending
		}
	}
	return responseOutcome{kind: responsePass}, pending
}

func retryAfter(headers http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(headers.Get("Retry-After"))
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64((24*time.Hour)/time.Second) {
			return 24 * time.Hour
		}
		return clampDuration(time.Duration(seconds) * time.Second)
	}
	if when, err := http.ParseTime(value); err == nil {
		return clampDuration(when.Sub(now))
	}
	return 30 * time.Second
}

func clampDuration(duration time.Duration) time.Duration {
	if duration < time.Second {
		return time.Second
	}
	if duration > 24*time.Hour {
		return 24 * time.Hour
	}
	return duration
}

func inspectResponse(body io.Reader, limit int64) ([]byte, bool) {
	if body == nil {
		return nil, true
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return data, false
	}
	return data, int64(len(data)) <= limit
}

func isQuotaResponse(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return quotaValue(value)
}

func quotaValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return quotaMapValue(typed)
	case []any:
		return quotaSliceValue(typed)
	}
	return false
}

func quotaMapValue(values map[string]any) bool {
	for key, child := range values {
		if quotaFieldValue(key, child) || quotaValue(child) {
			return true
		}
	}
	return false
}

func quotaSliceValue(values []any) bool {
	for _, child := range values {
		if quotaValue(child) {
			return true
		}
	}
	return false
}

func quotaFieldValue(key string, value any) bool {
	switch key {
	case "code", "type", "error_code", "status":
		text, ok := value.(string)
		return ok && quotaCode(text)
	default:
		return false
	}
}

func quotaCode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rate_limit_exceeded", "insufficient_quota", "quota_exceeded", "usage_limit_reached":
		return true
	default:
		return false
	}
}

func (proxy *Proxy) forwardResponse(writer http.ResponseWriter, response *http.Response, prefix []byte, accountID string, requestKeys affinityKeys, lifecycle *requestLifecycle) {
	if response == nil {
		return
	}
	defer response.Body.Close()
	stream := strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	if prefix == nil && !stream {
		prefix, _ = inspectResponse(response.Body, proxy.maxInspect)
	}
	if response.StatusCode < 400 {
		responseKeys := responseAffinity(response.Header, prefix, stream)
		_ = proxy.affinity.remember(accountID, mergeAffinityKeys(requestKeys, responseKeys), proxy.now())
	}
	copyResponseHeaders(writer.Header(), response.Header)
	for _, header := range []string{"Content-Type", "Content-Length", "Date"} {
		if !hasHeader(response.Header, header) {
			writer.Header()[header] = nil
		}
	}
	declareResponseTrailers(writer.Header(), response.Header, response.Trailer)
	lifecycle.commit()
	writer.WriteHeader(response.StatusCode)
	if stream {
		complete := proxy.forwardStream(writer, response.Body, prefix, accountID, requestKeys, response.Header)
		copyTrailers(writer.Header(), response.Header, response.Trailer)
		if complete {
			lifecycle.complete()
		} else {
			lifecycle.failAfterCommit()
		}
		return
	}
	if len(prefix) > 0 {
		if _, err := writer.Write(prefix); err != nil {
			lifecycle.failAfterCommit()
			return
		}
	}
	if !copyResponseBody(writer, response.Body) {
		lifecycle.failAfterCommit()
		return
	}
	copyTrailers(writer.Header(), response.Header, response.Trailer)
	lifecycle.complete()
}

func (proxy *Proxy) forwardStream(writer http.ResponseWriter, body io.Reader, prefix []byte, accountID string, requestKeys affinityKeys, headers http.Header) bool {
	forwarder := streamForwarder{
		proxy:     proxy,
		writer:    writer,
		accountID: accountID,
		headers:   headers,
		seen:      make([]byte, 0, len(prefix)),
	}
	return forwarder.forward(body, prefix)
}

type streamForwarder struct {
	proxy     *Proxy
	writer    http.ResponseWriter
	accountID string
	headers   http.Header
	seen      []byte
}

func (forwarder *streamForwarder) forward(body io.Reader, prefix []byte) bool {
	if forwarder.write(prefix) != nil {
		return false
	}
	buffer := make([]byte, 32*1024)
	for {
		read, err := body.Read(buffer)
		if read > 0 && forwarder.write(buffer[:read]) != nil {
			return false
		}
		if err != nil {
			return err == io.EOF
		}
	}
}

func (forwarder *streamForwarder) write(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	if _, err := forwarder.writer.Write(chunk); err != nil {
		return err
	}
	if flusher, ok := forwarder.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	forwarder.remember(chunk)
	return nil
}

func (forwarder *streamForwarder) remember(chunk []byte) {
	if len(forwarder.seen) >= int(forwarder.proxy.maxInspect) {
		return
	}
	remaining := int(forwarder.proxy.maxInspect) - len(forwarder.seen)
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	forwarder.seen = append(forwarder.seen, chunk...)
	responseKeys := responseAffinity(forwarder.headers, forwarder.seen, true)
	_ = forwarder.proxy.affinity.remember(forwarder.accountID, responseKeys, forwarder.proxy.now())
}

func copyResponseBody(writer http.ResponseWriter, reader io.Reader) bool {
	buffer := make([]byte, 32*1024)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			if _, writeErr := writer.Write(buffer[:read]); writeErr != nil {
				return false
			}
		}
		if err != nil {
			return err == io.EOF
		}
	}
}

func requestAffinity(request *http.Request, body []byte) affinityKeys {
	keys := affinityKeys{
		turn:    strings.TrimSpace(request.Header.Get("x-codex-turn-state")),
		session: firstHeader(request.Header, "x-codex-session-id", "x-session-id", "session-id"),
	}
	if len(body) == 0 {
		return keys
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return keys
	}
	keys.previous = objectString(object, "previous_response_id")
	if keys.session == "" {
		keys.session = objectString(object, "session_id", "conversation_id", "thread_id")
	}
	return keys
}

func responseAffinity(headers http.Header, body []byte, stream bool) affinityKeys {
	keys := affinityKeys{
		turn:    strings.TrimSpace(headers.Get("x-codex-turn-state")),
		session: firstHeader(headers, "x-codex-session-id", "x-session-id", "session-id"),
	}
	if len(body) == 0 {
		return keys
	}
	if stream {
		scanner := bufio.NewScanner(strings.NewReader(string(body)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var object map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &object) == nil {
				keys = mergeAffinityKeys(keys, responseObjectAffinity(object))
			}
		}
		return keys
	}
	var object map[string]any
	if json.Unmarshal(body, &object) == nil {
		return mergeAffinityKeys(keys, responseObjectAffinity(object))
	}
	return keys
}

func responseObjectAffinity(object map[string]any) affinityKeys {
	keys := affinityKeys{
		previous: objectString(object, "id", "response_id"),
		session:  objectString(object, "session_id", "conversation_id", "thread_id"),
		turn:     objectString(object, "turn_state"),
	}
	collectNestedAffinity(object, &keys, 0)
	return keys
}

func collectNestedAffinity(object map[string]any, keys *affinityKeys, depth int) {
	if depth >= 3 {
		return
	}
	for name, value := range object {
		nested, ok := nestedAffinityObject(name, value)
		if !ok {
			continue
		}
		mergeNestedAffinity(keys, nested)
		collectNestedAffinity(nested, keys, depth+1)
	}
}

func nestedAffinityObject(name string, value any) (map[string]any, bool) {
	switch name {
	case "response", "data", "result", "payload":
		nested, ok := value.(map[string]any)
		return nested, ok
	default:
		return nil, false
	}
}

func mergeNestedAffinity(keys *affinityKeys, nested map[string]any) {
	if keys.previous == "" {
		keys.previous = objectString(nested, "id", "response_id")
	}
	if keys.session == "" {
		keys.session = objectString(nested, "session_id", "conversation_id", "thread_id")
	}
	if keys.turn == "" {
		keys.turn = objectString(nested, "turn_state")
	}
}

func mergeAffinityKeys(first, second affinityKeys) affinityKeys {
	if first.previous == "" {
		first.previous = second.previous
	}
	if first.turn == "" {
		first.turn = second.turn
	}
	if first.session == "" {
		first.session = second.session
	}
	return first
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func objectString(object map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := object[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
