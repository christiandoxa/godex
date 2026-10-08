package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

func freshSessionUsageLimitProof04360(ctx context.Context, path, sessionID string) bool {
	return freshSessionRecoveryClass04360(ctx, path, sessionID) == "usage_limit"
}

func freshSessionRecoveryClass04360(ctx context.Context, path, sessionID string) string {
	if ctx.Err() != nil || !sessionentity.ValidID(sessionID) {
		return ""
	}
	name := filepath.Base(path)
	compressed := strings.HasSuffix(name, ".jsonl.zst")
	if !strings.HasPrefix(name, "rollout-") ||
		!(strings.HasSuffix(name, "-"+sessionID+".jsonl") ||
			strings.HasSuffix(name, "-"+sessionID+".jsonl.zst")) {
		return ""
	}
	var contents []byte
	if compressed {
		decoded, ok := readCompressedRecovery04360(path, recoveryScanCap04360)
		if !ok {
			return ""
		}
		contents = decoded
	} else {
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() || before.Size() > recoveryScanCap04360 {
			return ""
		}
		file, err := os.Open(path)
		if err != nil {
			return ""
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !os.SameFile(before, opened) {
			return ""
		}
		contents, err = io.ReadAll(io.LimitReader(file, recoveryScanCap04360+1))
		if err != nil || len(contents) > int(recoveryScanCap04360) {
			return ""
		}
	}
	if !bytes.HasSuffix(contents, []byte("\n")) || !hasFreshSessionMeta04360(contents, sessionID) {
		return ""
	}
	return scanRecoveryClassFragment04360(ctx, sessionID, contents)
}

func hasFreshSessionMeta04360(content []byte, sessionID string) bool {
	for _, line := range bytes.Split(content, []byte("\n")) {
		if len(line) == 0 || len(line) > recoveryLineCap04360 {
			continue
		}
		var record map[string]any
		if json.Unmarshal(line, &record) != nil || record["type"] != "session_meta" {
			continue
		}
		meta, _ := record["payload"].(map[string]any)
		if meta != nil {
			if id, _ := meta["id"].(string); id == sessionID {
				return true
			}
			if id, _ := meta["session_id"].(string); id == sessionID {
				return true
			}
		}
	}
	return false
}
