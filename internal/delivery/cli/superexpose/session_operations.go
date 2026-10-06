package superexpose

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	outputCursorVersion       = 1
	outputVerifyMaxLineBytes  = 512 * 1024
	outputVerificationMaxScan = 4 * 1024 * 1024
)

type sessionPromptWriteRequest struct {
	workspaceRoot string
	message       string
	cwd           string
	godexPID      *uint32
	threadID      string
	bindingKey    string
}

type sessionPreemptRequest struct {
	workspaceRoot string
	cwd           string
	godexPID      *uint32
	threadID      string
	bindingKey    string
}

func (service *existingSessionService) write(request sessionPromptWriteRequest) (map[string]any, error) {
	service.operation.Lock()
	defer service.operation.Unlock()

	workspace, err := canonicalSessionWorkspace(request.workspaceRoot, request.cwd)
	if err != nil {
		return nil, err
	}
	binding, err := service.binding(request.bindingKey)
	if err != nil {
		return nil, err
	}
	target, err := service.resolveSessionTarget(
		workspace, request.godexPID, request.threadID, binding,
	)
	if err != nil {
		return nil, err
	}
	var before *rolloutAnchor
	if path, err := service.outputSource(target); err == nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, sessionOutputSourceChanged
		}
		sourceID, sourceErr := outputSourceID(path, target.threadID)
		if sourceErr != nil {
			return nil, sourceErr
		}
		before = &rolloutAnchor{path: path, offset: uint64(info.Size()), sourceID: sourceID}
	} else if !errors.Is(err, sessionOutputSourceUnavailable) {
		return nil, err
	}
	target, err = service.revalidate(target, workspace)
	if err != nil {
		return nil, err
	}

	invocation := service.queue.queueOnce(target, request.message)
	requeued := invocation.outcome == queueRejected || invocation.outcome == queuePreflight
	if requeued {
		target, err = service.revalidate(target, workspace)
		if err != nil {
			return nil, err
		}
		invocation = service.queue.queueOnce(target, request.message)
	}
	verification, err := service.verifyQueueInvocation(request, workspace, target, before, invocation)
	if err != nil {
		return nil, err
	}
	cursor := ""
	if before != nil {
		cursor, _ = outputCursorAnchor(target, *before)
	}
	if err := service.rememberBinding(request.bindingKey, target, anchorSourceID(before)); err != nil {
		return nil, err
	}
	result := map[string]any{
		"status":               "written",
		"godex_pid":            target.godex.pid,
		"codex_pid":            target.writer.pid,
		"thread_id":            target.threadID,
		"message_id":           nil,
		"submission_id":        nil,
		"output_cursor":        nil,
		"queue_exit":           0,
		"verification":         verification,
		"recovery_generation":  0,
		"last_prompt_requeued": requeued,
		"requeue_reason":       nil,
	}
	if invocation.messageID != "" {
		result["message_id"] = invocation.messageID
	}
	if invocation.submissionID != "" {
		result["submission_id"] = invocation.submissionID
	}
	if cursor != "" {
		result["output_cursor"] = cursor
	}
	if invocation.exitCode != nil {
		result["queue_exit"] = *invocation.exitCode
	}
	if requeued {
		result["recovery_generation"] = 1
		result["requeue_reason"] = "definitely_not_accepted"
	}
	return result, nil
}

type rolloutAnchor struct {
	path     string
	offset   uint64
	sourceID string
}

func anchorSourceID(anchor *rolloutAnchor) string {
	if anchor == nil {
		return ""
	}
	return anchor.sourceID
}

func (service *existingSessionService) verifyQueueInvocation(
	request sessionPromptWriteRequest,
	workspace string,
	target resolvedSessionTarget,
	before *rolloutAnchor,
	invocation queueInvocation,
) (string, error) {
	switch invocation.outcome {
	case queueRejected:
		return "", sessionQueueFailed
	case queuePreflight:
		return "", sessionNotQueueAddressable
	case queueAmbiguous:
		return "", sessionWriteAmbiguous
	case queueAccepted:
		if invocation.queued {
			return "queue_pending_observed", nil
		}
		if err := service.waitForRolloutUserMessage(request, workspace, target, before); err != nil {
			return "", err
		}
		return "rollout_user_event_observed", nil
	default:
		return "", sessionVerificationInconclusive
	}
}

func (service *existingSessionService) waitForRolloutUserMessage(
	request sessionPromptWriteRequest,
	workspace string,
	target resolvedSessionTarget,
	before *rolloutAnchor,
) error {
	deadline := time.Now().Add(queueCommandTimeout)
	for {
		current, err := service.revalidate(target, workspace)
		if err != nil {
			return err
		}
		visible, err := service.rolloutUserMessageVisible(current, target.threadID, before, request.message)
		if err != nil {
			return err
		}
		if visible {
			persisted, err := service.queue.persistedThread(current.stateDB, current.threadID)
			if err != nil || !persisted {
				return sessionStaleTarget
			}
			return nil
		}
		if !time.Now().Before(deadline) {
			return sessionVerificationInconclusive
		}
		time.Sleep(sessionResolutionPoll)
	}
}

func (service *existingSessionService) rolloutUserMessageVisible(
	target resolvedSessionTarget,
	threadID string,
	before *rolloutAnchor,
	expected string,
) (bool, error) {
	path, err := service.outputSource(target)
	if errors.Is(err, sessionOutputSourceUnavailable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	offset, err := rolloutAnchorOffset(before, path, threadID)
	if err != nil {
		return false, err
	}
	visible, err := rolloutContainsExactUserMessage(path, offset, expected)
	if errors.Is(err, sessionOutputSourceUnavailable) {
		return false, nil
	}
	return visible, err
}

func rolloutAnchorOffset(before *rolloutAnchor, path, threadID string) (uint64, error) {
	if before == nil {
		return 0, nil
	}
	sourceID, err := outputSourceID(path, threadID)
	if err != nil {
		return 0, err
	}
	if filepath.Clean(before.path) != filepath.Clean(path) || sourceID != before.sourceID {
		return 0, sessionOutputSourceChanged
	}
	return before.offset, nil
}

func canonicalSessionWorkspace(root, cwd string) (string, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", sessionVerificationInconclusive
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", sessionVerificationInconclusive
	}
	if cwd != "" && !sameCanonicalPath(cwd, canonical) {
		return "", sessionStaleTarget
	}
	return canonical, nil
}

func (service *existingSessionService) preempt(request sessionPreemptRequest) (map[string]any, error) {
	service.operation.Lock()
	defer service.operation.Unlock()

	workspace, err := canonicalSessionWorkspace(request.workspaceRoot, request.cwd)
	if err != nil {
		return nil, err
	}
	binding, err := service.binding(request.bindingKey)
	if err != nil {
		return nil, err
	}
	target, err := service.resolveSessionTarget(workspace, request.godexPID, request.threadID, binding)
	if err != nil {
		return nil, err
	}
	target, err = service.revalidate(target, workspace)
	if err != nil {
		return nil, err
	}
	preempted, err := service.queue.preempt(target)
	if err != nil {
		return nil, err
	}
	generation := service.generation.Add(1)
	if err := service.rememberBinding(request.bindingKey, target, ""); err != nil {
		return nil, err
	}
	var turn any
	if preempted.currentTurnID != "" {
		turn = preempted.currentTurnID
	}
	return map[string]any{
		"status":                   "preempted",
		"preempted":                true,
		"godex_pid":                target.godex.pid,
		"codex_pid":                target.writer.pid,
		"thread_id":                target.threadID,
		"current_turn_id":          turn,
		"current_turn_found":       preempted.currentTurnID != "",
		"current_turn_interrupted": preempted.currentTurnInterrupted,
		"cancelled_submission_ids": preempted.cancelledSubmissionIDs,
		"cancelled_count":          len(preempted.cancelledSubmissionIDs),
		"remaining_submission_ids": preempted.remainingSubmissionIDs,
		"remaining_count":          len(preempted.remainingSubmissionIDs),
		"queue_empty_at_boundary":  preempted.queueEmptyAtBoundary,
		"session_ready":            preempted.sessionReady,
		"generation":               generation,
		"generation_boundary":      generation,
	}, nil
}

func outputCursorAnchor(target resolvedSessionTarget, anchor rolloutAnchor) (string, error) {
	if target.godex.birthIdentity == "" || target.writer.birthIdentity == "" {
		return "", sessionVerificationInconclusive
	}
	currentSource, err := outputSourceID(anchor.path, target.threadID)
	if err != nil || currentSource != anchor.sourceID {
		return "", sessionOutputSourceChanged
	}
	checkpoint, err := sourceCheckpointID(anchor.path, anchor.offset)
	if err != nil {
		return "", err
	}
	currentSource, err = outputSourceID(anchor.path, target.threadID)
	if err != nil || currentSource != anchor.sourceID {
		return "", sessionOutputSourceChanged
	}
	return encodeOutputCursor(outputCursor{
		version:  outputCursorVersion,
		godexPID: target.godex.pid, godexBirth: target.godex.birthIdentity,
		codexPID: target.writer.pid, codexBirth: target.writer.birthIdentity,
		threadID: target.threadID, sourceID: anchor.sourceID,
		offset: anchor.offset, checkpointID: checkpoint,
	})
}

type outputCursor struct {
	version      uint8
	godexPID     uint32
	godexBirth   string
	codexPID     uint32
	codexBirth   string
	threadID     string
	sourceID     string
	offset       uint64
	eventIndex   int
	checkpointID string
}

func encodeOutputCursor(cursor outputCursor) (string, error) {
	value := map[string]any{
		"version":       cursor.version,
		"godex_pid":     cursor.godexPID,
		"godex_birth":   cursor.godexBirth,
		"codex_pid":     cursor.codexPID,
		"codex_birth":   cursor.codexBirth,
		"thread_id":     cursor.threadID,
		"source_id":     cursor.sourceID,
		"offset":        cursor.offset,
		"event_index":   cursor.eventIndex,
		"checkpoint_id": cursor.checkpointID,
	}
	content, err := json.Marshal(value)
	if err != nil {
		return "", sessionOutputReadFailed
	}
	return base64.RawURLEncoding.EncodeToString(content), nil
}

func rolloutContainsExactUserMessage(path string, offset uint64, expected string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, sessionOutputSourceUnavailable
	}
	if !info.Mode().IsRegular() || offset > uint64(info.Size()) {
		return false, sessionOutputSourceChanged
	}
	file, err := os.Open(path)
	if err != nil {
		return false, sessionOutputSourceUnavailable
	}
	defer file.Close()
	if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
		return false, sessionOutputReadFailed
	}
	reader := bufio.NewReaderSize(io.LimitReader(file, outputVerificationMaxScan+1), 64*1024)
	scanned := 0
	for {
		line, readErr := reader.ReadString('\n')
		scanned += len(line)
		if scanned > outputVerificationMaxScan {
			return false, nil
		}
		if len(line) <= outputVerifyMaxLineBytes && exactUserMessageLine([]byte(line), expected) {
			return true, nil
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return false, nil
			}
			return false, sessionOutputReadFailed
		}
	}
}

func exactUserMessageLine(line []byte, expected string) bool {
	line = []byte(strings.TrimSuffix(string(line), "\n"))
	var value map[string]any
	if json.Unmarshal(line, &value) != nil {
		return false
	}
	payload, _ := value["payload"].(map[string]any)
	if payload == nil {
		return false
	}
	switch value["type"] {
	case "event_msg":
		return payload["type"] == "user_message" && payload["message"] == expected
	case "response_item":
		return responseItemExactUserMessage(payload) == expected
	default:
		return false
	}
}

func responseItemExactUserMessage(payload map[string]any) string {
	if payload["type"] != "message" || payload["role"] != "user" {
		return ""
	}
	items, _ := payload["content"].([]any)
	parts := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item != nil && item["type"] == "input_text" {
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
