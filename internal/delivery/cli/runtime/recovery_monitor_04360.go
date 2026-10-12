package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

const recoveryScanCap04360 int64 = 1 << 20
const recoveryLineCap04360 = 64 << 10
const recoveryCheckpointProbeBytes04360 int64 = 4096

// A checkpoint starts at the current logical end. Historical errors never
// authorize a new model turn. Prefix hashes also reject same-file rewrites.
type recoveryCheckpoint04360 struct {
	path         string
	origin       os.FileInfo
	offset       int64
	valid        bool
	compressed   bool
	baselineSHA  [32]byte
	metadataID   string
	metadataSeen bool
}

func captureRecoveryCheckpoint04360(path string) recoveryCheckpoint04360 {
	compressed := strings.HasSuffix(path, ".jsonl.zst")
	if !compressed && filepath.Ext(path) != ".jsonl" {
		return recoveryCheckpoint04360{}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return recoveryCheckpoint04360{}
	}
	if compressed {
		baseline, ok := readCompressedRecovery04360(path, recoveryCompressedBaselineCap04360)
		if !ok || len(baseline) == 0 || !bytes.HasSuffix(baseline, []byte("\n")) {
			return recoveryCheckpoint04360{}
		}
		metadataID, metadataSeen := recoverySessionMetaID04360(baseline)
		return recoveryCheckpoint04360{
			path: path, origin: info, offset: int64(len(baseline)), valid: true,
			compressed: true, baselineSHA: sha256.Sum256(baseline),
			metadataID: metadataID, metadataSeen: metadataSeen,
		}
	}
	source, opened, ok := openRecoveryRegularFile04360(path)
	if !ok {
		return recoveryCheckpoint04360{}
	}
	defer source.Close()
	baselineSHA, ok := recoveryPrefixHash04360(source, opened.Size())
	if !ok {
		return recoveryCheckpoint04360{}
	}
	metadataID, metadataSeen := recoverySessionMetaIDFromFile04360(source, opened.Size())
	return recoveryCheckpoint04360{
		path: path, origin: info, offset: opened.Size(), valid: true, baselineSHA: baselineSHA,
		metadataID: metadataID, metadataSeen: metadataSeen,
	}
}

func (checkpoint recoveryCheckpoint04360) newAcceptedUsageLimit04360(ctx context.Context, sessionID string) bool {
	return checkpoint.newAcceptedRecoveryClass04360(ctx, sessionID) == "usage_limit"
}

func (checkpoint recoveryCheckpoint04360) newAcceptedRecoveryClass04360(ctx context.Context, sessionID string) string {
	if !checkpoint.valid || ctx.Err() != nil || !sessionentity.ValidID(sessionID) ||
		!checkpoint.matchesSession04360(sessionID) {
		return ""
	}
	if checkpoint.compressed {
		body, ok := readCompressedRecovery04360(checkpoint.path,
			recoveryCompressedBaselineCap04360+recoveryScanCap04360)
		if !ok || int64(len(body)) <= checkpoint.offset ||
			int64(len(body))-checkpoint.offset > recoveryScanCap04360 ||
			sha256.Sum256(body[:checkpoint.offset]) != checkpoint.baselineSHA {
			return ""
		}
		return scanRecoveryClassFragment04360(ctx, sessionID, body[checkpoint.offset:])
	}
	source, _, ok := openRecoveryRegularFile04360(checkpoint.path)
	if !ok {
		return ""
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(checkpoint.origin, info) {
		return ""
	}
	size := info.Size()
	if size <= checkpoint.offset || size-checkpoint.offset > recoveryScanCap04360 {
		return ""
	}
	baselineSHA, ok := recoveryPrefixHash04360(source, checkpoint.offset)
	if !ok || baselineSHA != checkpoint.baselineSHA {
		return ""
	}
	fragment, err := io.ReadAll(io.NewSectionReader(source, checkpoint.offset, size-checkpoint.offset))
	if err != nil {
		return ""
	}
	return scanRecoveryClassFragment04360(ctx, sessionID, fragment)
}

func (checkpoint recoveryCheckpoint04360) matchesSession04360(sessionID string) bool {
	return !checkpoint.metadataSeen || strings.EqualFold(strings.TrimSpace(checkpoint.metadataID), strings.TrimSpace(sessionID))
}

func openRecoveryRegularFile04360(path string) (*os.File, os.FileInfo, bool) {
	named, err := os.Lstat(path)
	if err != nil || !named.Mode().IsRegular() {
		return nil, nil, false
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, nil, false
	}
	opened, err := source.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(named, opened) {
		_ = source.Close()
		return nil, nil, false
	}
	return source, named, true
}

func recoverySessionMetaIDFromFile04360(source *os.File, size int64) (string, bool) {
	if size <= 0 {
		return "", false
	}
	limit := size
	if limit > recoveryScanCap04360 {
		limit = recoveryScanCap04360
	}
	content, err := io.ReadAll(io.NewSectionReader(source, 0, limit))
	if err != nil {
		return "", false
	}
	return recoverySessionMetaID04360(content)
}

func recoverySessionMetaID04360(content []byte) (string, bool) {
	for _, line := range bytes.Split(content, []byte("\n")) {
		if len(line) == 0 || len(line) > recoveryLineCap04360 {
			continue
		}
		var value map[string]any
		if json.Unmarshal(line, &value) != nil || value["type"] != "session_meta" {
			continue
		}
		payload, _ := value["payload"].(map[string]any)
		for _, key := range []string{"id", "session_id"} {
			if id, ok := payload[key].(string); ok && strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id), true
			}
		}
	}
	return "", false
}

func recoveryPrefixHash04360(source *os.File, size int64) ([32]byte, bool) {
	if size < 0 {
		return [32]byte{}, false
	}
	hash := sha256.New()
	var encodedSize [8]byte
	binary.LittleEndian.PutUint64(encodedSize[:], uint64(size))
	_, _ = hash.Write(encodedSize[:])
	probe := size
	if probe > recoveryCheckpointProbeBytes04360 {
		probe = recoveryCheckpointProbeBytes04360
	}
	prefix := make([]byte, probe)
	if probe > 0 {
		read, err := source.ReadAt(prefix, 0)
		if err != nil && (err != io.EOF || int64(read) != probe) || int64(read) != probe {
			return [32]byte{}, false
		}
		_, _ = hash.Write(prefix)
	}
	if size > 0 {
		start := size - recoveryCheckpointProbeBytes04360
		if start < 0 {
			start = 0
		}
		window := make([]byte, size-start)
		read, err := source.ReadAt(window, start)
		if err != nil && (err != io.EOF || int64(read) != int64(len(window))) || read != len(window) {
			return [32]byte{}, false
		}
		_, _ = hash.Write(window)
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest, true
}

func scanRecoveryFragment04360(ctx context.Context, sessionID string, fragment []byte) bool {
	return scanRecoveryClassFragment04360(ctx, sessionID, fragment) == "usage_limit"
}

func scanRecoveryClassFragment04360(ctx context.Context, sessionID string, fragment []byte) string {
	if !bytes.HasSuffix(fragment, []byte("\n")) {
		return ""
	}
	accepted := false
	for _, line := range bytes.Split(fragment, []byte("\n")) {
		if ctx.Err() != nil {
			return ""
		}
		if len(line) == 0 {
			continue
		}
		if len(line) > recoveryLineCap04360 {
			return ""
		}
		var value map[string]any
		if json.Unmarshal(line, &value) != nil {
			continue
		}
		if !recordSession04360(value, sessionID) {
			continue
		}
		if recordUserAccepted04360(value) {
			accepted = true
			continue
		}
		if accepted {
			if class := structuredWorkflowRecoveryClass04360(value); class != "" {
				return class
			}
		}
		if recordTurnBoundary04360(value) {
			accepted = false
		}
	}
	return ""
}

// Like tagged Prodex's record_matches_session, nested routing metadata
// must never authorize recovery for another session. User-visible text is
// not a routing identity and is deliberately excluded from this traversal.
func recordSession04360(record map[string]any, sessionID string) bool {
	return recordSessionObject04360(record, sessionID, 0)
}

func recordSessionObject04360(record map[string]any, sessionID string, depth int) bool {
	if depth > 5 {
		return true
	}
	if record["type"] == "session_meta" {
		payload, _ := record["payload"].(map[string]any)
		for _, key := range []string{"id", "session_id"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" &&
				!strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(sessionID)) {
				return false
			}
		}
	}
	for _, key := range []string{"session_id", "sessionId", "thread_id", "threadId"} {
		if value, ok := record[key].(string); ok && value != sessionID {
			return false
		}
	}
	for key, value := range record {
		switch key {
		case "message", "content", "text", "delta":
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			if !recordSessionObject04360(nested, sessionID, depth+1) {
				return false
			}
		case []any:
			for _, entry := range nested {
				if obj, ok := entry.(map[string]any); ok &&
					!recordSessionObject04360(obj, sessionID, depth+1) {
					return false
				}
			}
		}
	}
	return true
}

func recordTurnBoundary04360(record map[string]any) bool {
	if record["method"] == "turn/started" {
		return true
	}
	if record["method"] == "turn/completed" {
		params, _ := record["params"].(map[string]any)
		turn, _ := params["turn"].(map[string]any)
		return turn == nil || turn["status"] != "failed"
	}
	typ, _ := record["type"].(string)
	payload, _ := record["payload"].(map[string]any)
	if typ == "event_msg" && (payload["type"] == "task_started" || payload["type"] == "turn_started") {
		return true
	}
	if typ == "event_msg" && (payload["type"] == "turn_aborted" || payload["type"] == "task_aborted" || payload["type"] == "turn_cancelled") {
		return true
	}
	if typ == "task_started" || typ == "turn_started" {
		return true
	}
	if typ == "turn_aborted" || typ == "task_aborted" || typ == "turn_cancelled" {
		return true
	}
	if typ == "event_msg" && (payload["type"] == "task_complete" || payload["type"] == "turn_complete") {
		return true
	}
	if typ == "task_complete" || typ == "turn_complete" {
		return true
	}
	if typ == "turn.completed" || typ == "turn_completed" {
		turn, _ := record["turn"].(map[string]any)
		if turn == nil {
			turn = record
		}
		return turn["status"] != "failed"
	}
	return false
}

func recordUserAccepted04360(record map[string]any) bool {
	typ, _ := record["type"].(string)
	payload, _ := record["payload"].(map[string]any)
	kind, _ := payload["type"].(string)
	role, _ := payload["role"].(string)
	return ((typ == "response_item" || typ == "message") && role == "user") ||
		(typ == "event_msg" && kind == "user_message")
}

func nestedError04360(value map[string]any) map[string]any {
	result, _ := value["error"].(map[string]any)
	return result
}
