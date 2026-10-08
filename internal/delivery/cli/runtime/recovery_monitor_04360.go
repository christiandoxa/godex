package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

const recoveryScanCap04360 int64 = 1 << 20
const recoveryLineCap04360 = 64 << 10

// A checkpoint starts at the current logical end. Historical errors
// never authorize a new model turn. Compressed, symlinked, missing or
// non-regular rollouts remain ineligible until their recovery contract is
// independently implemented.
type recoveryCheckpoint04360 struct {
	path   string
	origin os.FileInfo
	offset int64
	valid  bool
}

func captureRecoveryCheckpoint04360(path string) recoveryCheckpoint04360 {
	if filepath.Ext(path) != ".jsonl" {
		return recoveryCheckpoint04360{}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return recoveryCheckpoint04360{}
	}
	return recoveryCheckpoint04360{path: path, origin: info, offset: info.Size(), valid: true}
}

func (checkpoint recoveryCheckpoint04360) newAcceptedUsageLimit04360(ctx context.Context, sessionID string) bool {
	if !checkpoint.valid || ctx.Err() != nil || !sessionentity.ValidID(sessionID) {
		return false
	}
	source, err := os.Open(checkpoint.path)
	if err != nil {
		return false
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(checkpoint.origin, info) {
		return false
	}
	size := info.Size()
	if size <= checkpoint.offset || size-checkpoint.offset > recoveryScanCap04360 {
		return false
	}
	fragment, err := io.ReadAll(io.NewSectionReader(source, checkpoint.offset, size-checkpoint.offset))
	if err != nil || !bytes.HasSuffix(fragment, []byte("\n")) {
		return false
	}
	accepted := false
	for _, line := range bytes.Split(fragment, []byte("\n")) {
		if ctx.Err() != nil {
			return false
		}
		if len(line) == 0 {
			continue
		}
		if len(line) > recoveryLineCap04360 {
			return false
		}
		var value map[string]any
		if json.Unmarshal(line, &value) != nil {
			continue
		}
		if recordSession04360(value, sessionID) == false {
			continue
		}
		if recordUserAccepted04360(value) {
			accepted = true
			continue
		}
		if accepted && recordStructuredUsageLimit04360(value) {
			return true
		}
	}
	return false
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

func recordUserAccepted04360(record map[string]any) bool {
	typ, _ := record["type"].(string)
	payload, _ := record["payload"].(map[string]any)
	kind, _ := payload["type"].(string)
	role, _ := payload["role"].(string)
	return ((typ == "response_item" || typ == "message") && role == "user") ||
		(typ == "event_msg" && kind == "user_message")
}

func recordStructuredUsageLimit04360(record map[string]any) bool {
	kind, _ := record["type"].(string)
	var errBody map[string]any
	switch kind {
	case "error", "turn.failed", "turn_failed":
		errBody, _ = record["error"].(map[string]any)
	case "event_msg":
		payload, _ := record["payload"].(map[string]any)
		if payload["type"] != "error" {
			return false
		}
		errBody = payload
	default:
		return false
	}
	if errBody == nil {
		return false
	}
	for _, scope := range []map[string]any{errBody, nestedError04360(errBody)} {
		if scope == nil {
			continue
		}
		if code, _ := scope["code"].(string); code == "usage_limit_reached" || code == "usage_limit_exceeded" {
			return true
		}
		if name, _ := scope["codex_error_info"].(string); name == "usage_limit_exceeded" {
			return true
		}
		if info, ok := scope["codex_error_info"].(map[string]any); ok {
			if _, yes := info["usage_limit_exceeded"]; yes {
				return true
			}
		}
	}
	return false
}
func nestedError04360(value map[string]any) map[string]any {
	result, _ := value["error"].(map[string]any)
	return result
}
