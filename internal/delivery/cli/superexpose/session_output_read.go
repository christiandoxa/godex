package superexpose

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/redact"
)

const (
	outputCursorMaxBytes    = 16 * 1024
	outputReadMaxBytes      = 512 * 1024
	outputReadMaxLineBytes  = 64 * 1024
	outputReadMaxTextBytes  = 8 * 1024
	outputReadMaxTotalText  = 256 * 1024
	outputReadMaxWait       = 10 * time.Second
	outputReadPoll          = 100 * time.Millisecond
	outputReadMaxEventIndex = 65_536
)

type outputReadRequest struct {
	workspaceRoot string
	cursor        string
	limit         int
	waitMS        uint64
	godexPID      *uint32
	threadID      string
	bindingKey    string
}

type outputEvent struct {
	Sequence  uint64
	Timestamp string
	Kind      string
	Name      any
	Status    any
	Text      string
}

type outputReadBatch struct {
	events         []outputEvent
	nextOffset     uint64
	nextEventIndex int
	hasMore        bool
}

func decodeOutputCursor(value string) (outputCursor, error) {
	if value == "" || len(value) > outputCursorMaxBytes {
		return outputCursor{}, sessionInvalidCursor
	}
	content, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return outputCursor{}, sessionInvalidCursor
	}
	var raw map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if decoder.Decode(&raw) != nil {
		return outputCursor{}, sessionInvalidCursor
	}
	version, ok := rawUint(raw["version"])
	if !ok || version != outputCursorVersion {
		return outputCursor{}, sessionInvalidCursor
	}
	godexPID, ok := rawUint(firstAny(raw, "godex_pid", "prodex_pid"))
	if !ok || godexPID == 0 || godexPID > uint64(^uint32(0)) {
		return outputCursor{}, sessionInvalidCursor
	}
	codexPID, ok := rawUint(raw["codex_pid"])
	if !ok || codexPID == 0 || codexPID > uint64(^uint32(0)) {
		return outputCursor{}, sessionInvalidCursor
	}
	offset, ok := rawUint(raw["offset"])
	if !ok {
		return outputCursor{}, sessionInvalidCursor
	}
	eventIndex, ok := rawUint(raw["event_index"])
	if !ok || eventIndex > outputReadMaxEventIndex {
		return outputCursor{}, sessionInvalidCursor
	}
	cursor := outputCursor{
		version:    uint8(version),
		godexPID:   uint32(godexPID),
		codexPID:   uint32(codexPID),
		offset:     offset,
		eventIndex: int(eventIndex),
	}
	cursor.godexBirth, _ = firstAny(raw, "godex_birth", "prodex_birth").(string)
	cursor.codexBirth, _ = raw["codex_birth"].(string)
	cursor.threadID, _ = raw["thread_id"].(string)
	cursor.sourceID, _ = raw["source_id"].(string)
	cursor.checkpointID, _ = raw["checkpoint_id"].(string)
	if cursor.godexBirth == "" || cursor.codexBirth == "" || !canonicalUUID(cursor.threadID) ||
		cursor.sourceID == "" || cursor.checkpointID == "" {
		return outputCursor{}, sessionInvalidCursor
	}
	cursor.threadID = strings.ToLower(cursor.threadID)
	return cursor, nil
}

func firstAny(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func rawUint(value any) (uint64, bool) {
	switch current := value.(type) {
	case json.Number:
		parsed, err := current.Int64()
		return uint64(parsed), err == nil && parsed >= 0
	case float64:
		return uint64(current), current >= 0 && current == float64(uint64(current))
	default:
		return 0, false
	}
}

func (cursor outputCursor) matches(target resolvedSessionTarget, sourceID string) bool {
	return cursor.version == outputCursorVersion &&
		cursor.godexPID == target.godex.pid &&
		cursor.godexBirth == target.godex.birthIdentity &&
		cursor.codexPID == target.writer.pid &&
		cursor.codexBirth == target.writer.birthIdentity &&
		cursor.threadID == target.threadID &&
		cursor.sourceID == sourceID
}

func (service *existingSessionService) readOutput(request outputReadRequest) (map[string]any, error) {
	workspace, err := canonicalSessionWorkspace(request.workspaceRoot, "")
	if err != nil {
		return nil, err
	}
	var cursor *outputCursor
	if request.cursor != "" {
		decoded, err := decodeOutputCursor(request.cursor)
		if err != nil {
			return nil, err
		}
		cursor = &decoded
	}
	var binding *sessionBinding
	if cursor == nil {
		binding, err = service.binding(request.bindingKey)
		if err != nil {
			return nil, err
		}
	}
	requestedPID := request.godexPID
	if requestedPID == nil && cursor != nil {
		value := cursor.godexPID
		requestedPID = &value
	}
	if requestedPID == nil && binding != nil {
		value := binding.target.godex.pid
		requestedPID = &value
	}
	target, err := service.resolveTargetForRequest(
		workspace, requestedPID, request.threadID, cursor != nil || binding != nil,
	)
	if errors.Is(err, sessionNoSession) && cursor != nil {
		return nil, sessionStaleCursor
	}
	if errors.Is(err, sessionNoSession) && (cursor != nil || binding != nil || request.godexPID != nil || request.threadID != "") {
		return nil, sessionStaleTarget
	}
	if err != nil {
		return nil, err
	}
	if err := service.verifyBinding(binding, target); err != nil {
		return nil, err
	}
	if request.threadID != "" && request.threadID != target.threadID {
		return nil, sessionStaleTarget
	}
	path, err := service.outputSource(target)
	if err != nil {
		return nil, err
	}
	sourceID, err := outputSourceID(path, target.threadID)
	if err != nil {
		return nil, err
	}
	if binding != nil && binding.sourceID != "" && binding.sourceID != sourceID {
		return nil, sessionOutputSourceChanged
	}
	offset, eventIndex := uint64(0), 0
	if cursor != nil {
		checkpoint, err := sourceCheckpointID(path, cursor.offset)
		if err != nil || checkpoint != cursor.checkpointID {
			return nil, sessionOutputSourceChanged
		}
		if !cursor.matches(target, sourceID) {
			return nil, sessionStaleCursor
		}
		offset, eventIndex = cursor.offset, cursor.eventIndex
	}
	limit := request.limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	wait := time.Duration(request.waitMS) * time.Millisecond
	if wait > outputReadMaxWait {
		wait = outputReadMaxWait
	}
	deadline := time.Now().Add(wait)
	for {
		batch, err := readOutputEvents(path, offset, eventIndex, limit)
		if err != nil {
			return nil, err
		}
		offset, eventIndex = batch.nextOffset, batch.nextEventIndex
		if len(batch.events) > 0 || wait == 0 || !time.Now().Before(deadline) {
			checkpoint, err := sourceCheckpointID(path, offset)
			if err != nil {
				return nil, err
			}
			nextCursor, err := encodeOutputCursor(outputCursor{
				version:  outputCursorVersion,
				godexPID: target.godex.pid, godexBirth: target.godex.birthIdentity,
				codexPID: target.writer.pid, codexBirth: target.writer.birthIdentity,
				threadID: target.threadID, sourceID: sourceID,
				offset: offset, eventIndex: eventIndex, checkpointID: checkpoint,
			})
			if err != nil {
				return nil, err
			}
			if cursor == nil {
				if err := service.rememberBinding(request.bindingKey, target, sourceID); err != nil {
					return nil, err
				}
			}
			return map[string]any{
				"status": "ok", "godex_pid": target.godex.pid, "codex_pid": target.writer.pid,
				"thread_id": target.threadID, "source": "codex_rollout",
				"events": outputEventsJSON(batch.events), "next_cursor": nextCursor, "has_more": batch.hasMore,
			}, nil
		}
		sleep := outputReadPoll
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep > 0 {
			time.Sleep(sleep)
		}
		target, err = service.revalidate(target, workspace)
		if err != nil {
			return nil, err
		}
		currentPath, err := service.outputSource(target)
		if err != nil {
			return nil, err
		}
		currentSource, err := outputSourceID(currentPath, target.threadID)
		if err != nil || currentSource != sourceID || currentPath != path {
			return nil, sessionOutputSourceChanged
		}
	}
}

func outputEventsJSON(events []outputEvent) []map[string]any {
	result := make([]map[string]any, 0, len(events))
	for _, event := range events {
		result = append(result, map[string]any{
			"sequence": event.Sequence, "timestamp": event.Timestamp, "kind": event.Kind,
			"name": event.Name, "status": event.Status, "text": event.Text,
		})
	}
	return result
}

func readOutputEvents(path string, offset uint64, eventIndex, limit int) (outputReadBatch, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || offset > uint64(info.Size()) {
		return outputReadBatch{}, sessionOutputSourceChanged
	}
	file, err := os.Open(path)
	if err != nil {
		return outputReadBatch{}, sessionOutputReadFailed
	}
	defer file.Close()
	if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
		return outputReadBatch{}, sessionOutputReadFailed
	}
	content, err := io.ReadAll(io.LimitReader(file, outputReadMaxBytes+1))
	if err != nil {
		return outputReadBatch{}, sessionOutputReadFailed
	}
	sourceLen := uint64(info.Size())
	completeEnd := bytes.LastIndexByte(content, '\n')
	if completeEnd < 0 {
		if len(content) > outputReadMaxBytes {
			next, err := skipOversizedOutputLine(path, offset, sourceLen)
			if err != nil {
				return outputReadBatch{}, err
			}
			if next > offset {
				return outputReadBatch{
					events:     []outputEvent{gapOutputEvent(offset, "oversized_record")},
					nextOffset: next, hasMore: next < sourceLen,
				}, nil
			}
		}
		return outputReadBatch{nextOffset: offset, nextEventIndex: eventIndex, hasMore: offset < sourceLen}, nil
	}
	complete := content[:completeEnd+1]
	events := make([]outputEvent, 0, min(limit, 16))
	totalText := 0
	consumed := 0
	for len(complete) > 0 {
		newline := bytes.IndexByte(complete, '\n')
		line := complete[:newline+1]
		lineStart := offset + uint64(consumed)
		startIndex := 0
		if consumed == 0 {
			startIndex = eventIndex
		}
		if len(events) >= limit {
			return outputReadBatch{events: events, nextOffset: lineStart, nextEventIndex: startIndex, hasMore: true}, nil
		}
		if len(line) > outputReadMaxLineBytes && !rawVisibleUserMessage(line) {
			events = append(events, gapOutputEvent(lineStart, "oversized_record"))
			consumed += len(line)
			complete = complete[newline+1:]
			continue
		}
		if !utf8.Valid(bytes.TrimSuffix(line, []byte{'\n'})) {
			events = append(events, gapOutputEvent(lineStart, "invalid_utf8"))
			consumed += len(line)
			complete = complete[newline+1:]
			continue
		}
		parsed, valid := transcriptOutputEvents(line)
		if !valid {
			events = append(events, gapOutputEvent(lineStart, "malformed_record"))
			consumed += len(line)
			complete = complete[newline+1:]
			continue
		}
		if startIndex > len(parsed) {
			return outputReadBatch{}, sessionOutputSourceChanged
		}
		for index := startIndex; index < len(parsed); index++ {
			if len(events) >= limit || totalText+len(parsed[index].Text) > outputReadMaxTotalText {
				return outputReadBatch{events: events, nextOffset: lineStart, nextEventIndex: index, hasMore: true}, nil
			}
			event := parsed[index]
			event.Sequence = lineStart*65_536 + uint64(index)
			totalText += len(event.Text)
			events = append(events, event)
		}
		consumed += len(line)
		complete = complete[newline+1:]
	}
	next := offset + uint64(consumed)
	return outputReadBatch{events: events, nextOffset: next, hasMore: next < sourceLen}, nil
}

func skipOversizedOutputLine(path string, offset, sourceLen uint64) (uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, sessionOutputReadFailed
	}
	defer file.Close()
	if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
		return 0, sessionOutputReadFailed
	}
	buffer := make([]byte, 64*1024)
	skipped := uint64(0)
	for skipped < outputSkipMaxBytes {
		remaining := outputSkipMaxBytes - skipped
		chunk := buffer
		if uint64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		count, err := file.Read(chunk)
		if count > 0 {
			if index := bytes.IndexByte(chunk[:count], '\n'); index >= 0 {
				return offset + skipped + uint64(index+1), nil
			}
			skipped += uint64(count)
		}
		if errors.Is(err, io.EOF) {
			return offset, nil
		}
		if err != nil {
			return 0, sessionOutputReadFailed
		}
	}
	if offset+skipped >= sourceLen {
		return offset, nil
	}
	return 0, sessionRecoveryFailed
}

func gapOutputEvent(sequence uint64, reason string) outputEvent {
	return outputEvent{
		Sequence: sequence * 65_536, Timestamp: "-", Kind: "gap",
		Name: reason, Status: "skipped", Text: "output gap: " + reason + "; record omitted",
	}
}

func transcriptOutputEvents(line []byte) ([]outputEvent, bool) {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	var record map[string]any
	if json.Unmarshal(line, &record) != nil {
		return nil, false
	}
	recordType, _ := record["type"].(string)
	payload, _ := record["payload"].(map[string]any)
	if recordType == "" || payload == nil {
		return nil, false
	}
	timestamp, _ := record["timestamp"].(string)
	if timestamp == "" {
		timestamp = "-"
	}
	var events []outputEvent
	switch recordType {
	case "event_msg":
		if event, ok := eventMessageOutput(timestamp, payload); ok {
			events = append(events, event)
		}
	case "response_item":
		if event, ok := responseItemOutput(timestamp, payload); ok {
			events = append(events, event)
		}
	case "session_meta", "turn_context":
	default:
	}
	return events, true
}

func eventMessageOutput(timestamp string, payload map[string]any) (outputEvent, bool) {
	eventType, _ := payload["type"].(string)
	switch eventType {
	case "user_message":
		return visibleOutput(timestamp, "user", nil, nil, sessionTextValue(payload["message"]))
	case "agent_message":
		return visibleOutput(timestamp, "assistant", nil, nil, sessionTextValue(payload["message"]))
	case "agent_reasoning":
		return outputEvent{}, false
	}
	if source := protocolEventSource(eventType); source != "" {
		return visibleOutput(timestamp, source, nil, nil, protocolEventText(eventType, payload))
	}
	if statusEvent(eventType) {
		kind := "terminal"
		lower := strings.ToLower(eventType + " " + sessionTextValue(payload["status"]))
		if strings.Contains(lower, "fail") || strings.Contains(lower, "abort") || strings.Contains(lower, "error") {
			kind = "error"
		}
		return visibleOutput(timestamp, kind, nil, nil, statusEventText(eventType, payload))
	}
	return outputEvent{}, false
}

func responseItemOutput(timestamp string, payload map[string]any) (outputEvent, bool) {
	itemType := sessionTextValue(payload["type"])
	switch itemType {
	case "message":
		role := sessionTextValue(payload["role"])
		if role != "user" && role != "assistant" {
			return outputEvent{}, false
		}
		if role == "user" && !visibleUserMessage(payload) {
			return outputEvent{}, false
		}
		return visibleOutput(timestamp, role, nil, nil, contentText(payload["content"]))
	case "function_call":
		return visibleOutput(timestamp, "tool", sanitizeToolName(sessionTextValue(payload["name"])), "started", sessionTextValue(payload["arguments"]))
	case "function_call_output", "custom_tool_call_output":
		return visibleOutput(timestamp, "tool", nil, "completed", sessionTextValue(payload["output"]))
	case "custom_tool_call":
		return visibleOutput(timestamp, "tool", sanitizeToolName(sessionTextValue(payload["name"])), "started", sessionTextValue(payload["input"]))
	case "local_shell_call", "shell_call":
		name := sanitizeToolName(sessionTextValue(payload["name"]))
		if name == "tool" {
			name = "shell"
		}
		return visibleOutput(timestamp, "tool", name, "started", jsonText(firstAny(payload, "command", "arguments", "action")))
	case "local_shell_call_output", "shell_call_output":
		return visibleOutput(timestamp, "tool", nil, "completed", jsonText(firstAny(payload, "output", "aggregated_output", "stdout")))
	case "reasoning":
		return outputEvent{}, false
	default:
		source := protocolItemSource(itemType)
		if source == "" {
			return outputEvent{}, false
		}
		return visibleOutput(timestamp, source, nil, nil, protocolEventText(itemType, payload))
	}
}

func visibleOutput(timestamp, kind string, name, status any, text string) (outputEvent, bool) {
	text = sanitizeOutputText(redact.Secrets(text))
	if strings.TrimSpace(text) == "" {
		return outputEvent{}, false
	}
	return outputEvent{Timestamp: timestamp, Kind: kind, Name: name, Status: status, Text: text}, true
}

func sanitizeOutputText(value string) string {
	var builder strings.Builder
	for _, current := range value {
		if current < 0x20 && current != '\n' && current != '\r' && current != '\t' ||
			(current >= 0x7f && current <= 0x9f) {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(current)
		}
	}
	text := builder.String()
	if len(text) <= outputReadMaxTextBytes {
		return text
	}
	const marker = " …[text_truncated]"
	limit := outputReadMaxTextBytes - len(marker)
	for limit > 0 && !utf8Boundary(text, limit) {
		limit--
	}
	return text[:limit] + marker
}

func visibleUserMessage(payload map[string]any) bool {
	meta, _ := payload["internal_chat_message_metadata_passthrough"].(map[string]any)
	if meta == nil {
		return true
	}
	kinds, ok := meta["content_item_kinds"].([]any)
	if !ok || len(kinds) == 0 {
		return false
	}
	for _, kind := range kinds {
		if kind != "user.text" {
			return false
		}
	}
	return true
}

func contentText(raw any) string {
	items, _ := raw.([]any)
	parts := make([]string, 0, len(items))
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if item == nil {
			continue
		}
		if text, ok := firstAny(item, "text", "content").(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func rawVisibleUserMessage(line []byte) bool {
	if len(line) > outputVerifyMaxLineBytes {
		return false
	}
	var record map[string]any
	if json.Unmarshal(bytes.TrimSuffix(line, []byte{'\n'}), &record) != nil {
		return false
	}
	payload, _ := record["payload"].(map[string]any)
	if payload == nil {
		return false
	}
	switch record["type"] {
	case "event_msg":
		return payload["type"] == "user_message" && sessionTextValue(payload["message"]) != ""
	case "response_item":
		return responseItemExactUserMessage(payload) != ""
	default:
		return false
	}
}

func protocolEventSource(eventType string) string {
	lower := strings.ToLower(eventType)
	switch {
	case strings.Contains(lower, "mcp"):
		return "mcp"
	case strings.Contains(lower, "subagent"), strings.Contains(lower, "sub_agent"):
		return "agent"
	case strings.Contains(lower, "tool"), strings.Contains(lower, "computer"),
		strings.Contains(lower, "web_search"), strings.Contains(lower, "file_search"),
		strings.Contains(lower, "code_interpreter"):
		return "tool"
	default:
		return ""
	}
}

func protocolItemSource(itemType string) string {
	return protocolEventSource(itemType)
}

func protocolEventText(eventType string, payload map[string]any) string {
	details := make([]string, 0, 5)
	for _, entry := range []struct {
		keys  []string
		label string
	}{
		{[]string{"server", "server_label", "server_name"}, "server"},
		{[]string{"tool", "tool_name"}, "tool"},
		{[]string{"name"}, "name"},
		{[]string{"status"}, "status"},
		{[]string{"phase"}, "phase"},
	} {
		for _, key := range entry.keys {
			if value := operationValue(sessionTextValue(payload[key])); value != "" {
				details = append(details, entry.label+"="+value)
				break
			}
		}
	}
	if len(details) == 0 {
		return strings.ReplaceAll(eventType, "_", " ")
	}
	return strings.Join(details, " ")
}

func statusEvent(eventType string) bool {
	lower := strings.ToLower(eventType)
	switch lower {
	case "turn_completed", "turn_cancelled", "turn_interrupted", "turn_failed",
		"command_execution_started", "command_execution_completed", "command_execution_finished",
		"command_execution_output", "exec_command_begin", "exec_command_end", "error":
		return true
	}
	return strings.Contains(lower, "command") && strings.Contains(lower, "status")
}

func statusEventText(eventType string, payload map[string]any) string {
	details := make([]string, 0, 9)
	for _, key := range []string{"status", "exit_code", "exit_status", "reason", "duration_ms", "message"} {
		if value := jsonText(payload[key]); value != "" {
			details = append(details, key+"="+value)
		}
	}
	for _, key := range []string{"stdout", "stderr", "output"} {
		if value := jsonText(payload[key]); strings.TrimSpace(value) != "" {
			details = append(details, key+":\n"+value)
		}
	}
	if len(details) == 0 {
		return strings.ReplaceAll(eventType, "_", " ")
	}
	return strings.Join(details, " ")
}

func operationValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	for _, current := range value {
		if current <= 0x1f || current >= 0x7f && current <= 0x9f {
			return ""
		}
	}
	runes := []rune(value)
	if len(runes) > 192 {
		runes = runes[:192]
	}
	return string(runes)
}

func sanitizeToolName(value string) string {
	value = redact.Secrets(value)
	if value == "" {
		return "tool"
	}
	var builder strings.Builder
	count := 0
	for _, current := range value {
		if count >= 128 {
			break
		}
		if current <= 0x7f &&
			(current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' ||
				current >= '0' && current <= '9' || strings.ContainsRune("_-.:", current)) {
			builder.WriteRune(current)
		} else {
			builder.WriteByte('_')
		}
		count++
	}
	if builder.Len() == 0 {
		return "tool"
	}
	return builder.String()
}

func jsonText(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case json.Number:
		return current.String()
	case float64, bool:
		content, _ := json.Marshal(current)
		return string(content)
	case []any, map[string]any:
		content, _ := json.Marshal(current)
		return string(content)
	default:
		return ""
	}
}

func sessionTextValue(value any) string {
	text, _ := value.(string)
	return text
}
