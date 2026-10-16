package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
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
		if err != nil || !info.Mode().IsRegular() ||
			runtime.GOOS != "windows" && info.Mode().Perm()&0o002 != 0 {
			return -1
		}
		data, ok := readCompressedRecovery04360(path, recoveryCompressedBaselineCap04360)
		if !ok {
			return -1
		}
		// A compressed native rollout may be rewritten while the hook is
		// delivered. Anchor the cursor to a complete JSONL record, not
		// an unfinished trailing event.
		complete := bytes.LastIndexByte(data, '\n') + 1
		if complete == 0 {
			return -1
		}
		data = data[:complete]
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
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o002 != 0 {
		return -1
	}
	opened, err := file.Stat()
	if err != nil {
		return -1
	}
	complete := sessionStartLastCompleteJSONLOffset04379(file, opened.Size())
	if complete < 0 {
		return -1
	}
	if metadataID, seen := recoverySessionMetaIDFromFile04360(file, complete); seen &&
		!strings.EqualFold(metadataID, sessionID) {
		return -1
	}
	return complete
}

// A partially written JSONL event must not poison the initial recovery
// checkpoint. Cap backward inspection of the active tail; if no complete
// record exists within that bound, decline the cursor instead of guessing.
func sessionStartLastCompleteJSONLOffset04379(file *os.File, size int64) int64 {
	if size <= 0 {
		return -1
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], size-1); err != nil {
		return -1
	}
	if last[0] == '\n' {
		return size
	}
	const lookback = int64(1 << 20)
	start := max(int64(0), size-lookback)
	tail := make([]byte, int(size-start))
	if _, err := file.ReadAt(tail, start); err != nil {
		return -1
	}
	end := bytes.LastIndexByte(tail, '\n')
	if end < 0 {
		return -1
	}
	return start + int64(end+1)
}

func sessionStartOffsetPath04379(marker string) string {
	return strings.TrimSuffix(marker, ".id") + ".offset"
}

const sessionStartRolloutPathLimit04379 = 4095

func sessionStartRolloutSidecarPath04379(marker string) string {
	return strings.TrimSuffix(marker, ".id") + ".rollout"
}

// Read the two pieces of callback evidence under the same OS lock. The
// parameterless variant is used by marker fixtures; live recovery requires
// a matching native transcript, preventing a cursor from being replayed
// against another rollout file with the same UUID.
func sessionStartReadOffset04379(ctx context.Context, marker, sessionID string) (int64, bool) {
	return sessionStartReadOffsetForRollout04379(ctx, marker, sessionID, "")
}

func sessionStartReadOffsetForRollout04379(
	ctx context.Context, marker, sessionID, expectedRollout string,
) (int64, bool) {
	release, err := lockfile.Acquire(ctx, marker+".lock")
	if err != nil {
		return 0, false
	}
	defer release()
	current, err := readSessionStartMarker04379(marker)
	if err != nil || current != sessionID {
		return 0, false
	}
	if expectedRollout != "" {
		if !sessionStartRolloutPath04379(expectedRollout, sessionID) {
			return 0, false
		}
		binding := sessionStartRolloutSidecarPath04379(marker)
		info, err := os.Lstat(binding)
		if err != nil || !info.Mode().IsRegular() ||
			info.Size() < 2 || info.Size() > sessionStartRolloutPathLimit04379+1 {
			return 0, false
		}
		body, err := os.ReadFile(binding)
		if err != nil || len(body) < 2 || body[len(body)-1] != '\n' ||
			strings.TrimSuffix(string(body), "\n") != filepath.Clean(expectedRollout) {
			return 0, false
		}
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
