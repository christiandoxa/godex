package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

const recoveryScanCap04360 int64 = 1 << 20
const recoveryLineCap04360 = 64 << 10

// A checkpoint starts at the current logical end. Historical errors
// never authorize a new model turn. Compressed, symlinked, missing or
// non-regular rollouts remain ineligible until their recovery contract is
// independently implemented.
type recoveryCheckpoint04360 struct {
	path        string
	origin      os.FileInfo
	offset      int64
	valid       bool
	compressed  bool
	baselineSHA [32]byte
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
		return recoveryCheckpoint04360{
			path: path, origin: info, offset: int64(len(baseline)), valid: true,
			compressed: true, baselineSHA: sha256.Sum256(baseline),
		}
	}
	return recoveryCheckpoint04360{path: path, origin: info, offset: info.Size(), valid: true}
}

func (checkpoint recoveryCheckpoint04360) newAcceptedUsageLimit04360(ctx context.Context, sessionID string) bool {
	return checkpoint.newAcceptedRecoveryClass04360(ctx, sessionID) == "usage_limit"
}

func (checkpoint recoveryCheckpoint04360) newAcceptedRecoveryClass04360(ctx context.Context, sessionID string) string {
	if !checkpoint.valid || ctx.Err() != nil || !sessionentity.ValidID(sessionID) {
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
	source, err := os.Open(checkpoint.path)
	if err != nil {
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
	fragment, err := io.ReadAll(io.NewSectionReader(source, checkpoint.offset, size-checkpoint.offset))
	if err != nil {
		return ""
	}
	return scanRecoveryClassFragment04360(ctx, sessionID, fragment)
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
		if recordTurnBoundary04360(value) {
			accepted = false
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
	if typ == "event_msg" && payload["type"] == "turn_started" {
		return true
	}
	if typ == "turn.completed" || typ == "turn_completed" || typ == "task_complete" {
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
