package runtime

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

func sessionStartRolloutPath04379(path, sessionID string) bool {
	if !sessionentity.ValidID(sessionID) || !filepath.IsAbs(path) {
		return false
	}
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasPrefix(name, "rollout-") {
		return false
	}
	suffix := "-" + strings.ToLower(sessionID) + ".jsonl"
	return strings.HasSuffix(name, suffix) || strings.HasSuffix(name, suffix+".zst")
}

// SessionStart may include the Codex-owned transcript path. Snapshot its
// logical cursor before publishing the root marker, so a failure arriving
// before the first monitor poll cannot be lost. Untrusted paths fail closed.
func sessionStartTranscriptOffset04379(fields map[string]any, sessionID string) int64 {
	path, _ := fields["transcript_path"].(string)
	if !sessionStartRolloutPath04379(path, sessionID) {
		return -1
	}
	if strings.HasSuffix(strings.ToLower(path), ".jsonl.zst") {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o002 != 0 {
			return -1
		}
		data, ok := readCompressedRecovery04360(path, recoveryCompressedBaselineCap04360)
		if !ok {
			return -1
		}
		if metadataID, seen := recoverySessionMetaID04360(data); seen &&
			!strings.EqualFold(metadataID, sessionID) {
			return -1
		}
		return int64(len(data))
	}
	file, info, ok := openRecoveryRegularFile04360(path)
	if !ok {
		return -1
	}
	defer file.Close()
	if info.Mode().Perm()&0o002 != 0 {
		return -1
	}
	opened, err := file.Stat()
	if err != nil {
		return -1
	}
	if metadataID, seen := recoverySessionMetaIDFromFile04360(file, opened.Size()); seen &&
		!strings.EqualFold(metadataID, sessionID) {
		return -1
	}
	return opened.Size()
}

func sessionStartOffsetPath04379(marker string) string {
	return strings.TrimSuffix(marker, ".id") + ".offset"
}

// The marker and offset are read under the hook-writer lock so an in-flight
// root change cannot pair the old identity with a new cursor.
func sessionStartReadOffset04379(ctx context.Context, marker, sessionID string) (int64, bool) {
	release, err := lockfile.Acquire(ctx, marker+".lock")
	if err != nil {
		return 0, false
	}
	defer release()
	current, err := readSessionStartMarker04379(marker)
	if err != nil || current != sessionID {
		return 0, false
	}
	path := sessionStartOffsetPath04379(marker)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 2 || info.Size() > 21 {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return value, err == nil && value >= 0
}

// Hash the stored logical prefix, not the file's current end. A newer tail
// can then be inspected once even when it arrived before monitor startup.
func captureRecoveryCheckpointAt04379(path string, offset int64) recoveryCheckpoint04360 {
	current := captureRecoveryCheckpoint04360(path)
	if !current.valid || offset < 0 || offset >= current.offset {
		return current
	}
	if current.compressed {
		data, ok := readCompressedRecovery04360(path, recoveryCompressedBaselineCap04360)
		if !ok || !cursorEndsAtLine04379(data, offset) {
			return current
		}
		current.baselineSHA = sha256.Sum256(data[:offset])
		current.metadataID, current.metadataSeen = recoverySessionMetaID04360(data[:offset])
	} else {
		file, info, ok := openRecoveryRegularFile04360(path)
		if !ok {
			return current
		}
		defer file.Close()
		if !os.SameFile(current.origin, info) {
			return current
		}
		if offset > 0 {
			var last [1]byte
			if _, err := file.ReadAt(last[:], offset-1); err != nil || last[0] != '\n' {
				return current
			}
		}
		sum, ok := recoveryPrefixHash04360(file, offset)
		if !ok {
			return current
		}
		current.baselineSHA = sum
		current.metadataID, current.metadataSeen = recoverySessionMetaIDFromFile04360(file, offset)
	}
	current.offset = offset
	return current
}

func cursorEndsAtLine04379(data []byte, offset int64) bool {
	return offset >= 0 && offset <= int64(len(data)) &&
		(offset == 0 || data[offset-1] == '\n')
}
