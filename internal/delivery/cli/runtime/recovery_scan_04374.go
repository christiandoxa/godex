package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/klauspost/compress/zstd"
)

// scanAcceptedRecovery04360 consumes complete appended records while retaining
// acceptance state between live monitor polls. The child-exit path keeps its
// stricter whole-fragment scan and bounded replay behavior.
func (checkpoint *recoveryCheckpoint04360) scanAcceptedRecovery04360(
	ctx context.Context, sessionID string, consumed int64, accepted bool,
) (string, int64, bool, bool) {
	if checkpoint == nil || !checkpoint.valid || ctx.Err() != nil || !sessionentity.ValidID(sessionID) || consumed < 0 {
		return "", consumed, accepted, false
	}
	if !checkpoint.matchesSession04360(sessionID) {
		return "", consumed, false, true
	}
	fragment, ok, rebased := checkpoint.readAppendedRecovery04360(ctx, sessionID, consumed)
	if !ok {
		return "", consumed, accepted, false
	}
	if rebased {
		class, _, accepted := scanRecoveryClassSince04360(ctx, sessionID, fragment, false)
		return class, 0, accepted, true
	}
	class, advanced, accepted := scanRecoveryClassSince04360(ctx, sessionID, fragment, accepted)
	return class, consumed + advanced, accepted, true
}

func (checkpoint *recoveryCheckpoint04360) readAppendedRecovery04360(
	ctx context.Context, sessionID string, consumed int64,
) ([]byte, bool, bool) {
	if ctx.Err() != nil || consumed < 0 {
		return nil, false, false
	}
	if checkpoint.compressed {
		fragment, ok := readCompressedRecoverySince04360(*checkpoint, consumed)
		return fragment, ok, false
	}
	source, named, ok := openRecoveryRegularFile04360(checkpoint.path)
	if !ok {
		return nil, false, false
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(checkpoint.origin, info) {
		return nil, false, false
	}
	size := info.Size()
	start := checkpoint.offset + consumed
	if start < checkpoint.offset || start > size {
		if start > size && os.SameFile(checkpoint.origin, named) {
			// A same-file shrink may be compaction. Scan only after an explicit
			// compaction marker; otherwise rebase at the current logical end.
			fragment, ok := readCompactedRecoveryTail04360(source, size)
			if !ok {
				return nil, false, false
			}
			baselineSHA, ok := recoveryPrefixHash04360(source, size)
			if !ok {
				return nil, false, false
			}
			checkpoint.origin = named
			checkpoint.offset = size
			checkpoint.baselineSHA = baselineSHA
			checkpoint.metadataID, checkpoint.metadataSeen = recoverySessionMetaIDFromFile04360(source, size)
			return fragment, checkpoint.matchesSession04360(sessionID), true
		}
		return nil, false, false
	}
	baselineSHA, ok := recoveryPrefixHash04360(source, checkpoint.offset)
	if !ok || baselineSHA != checkpoint.baselineSHA {
		return nil, false, false
	}
	available := size - start
	if available > recoveryScanCap04360 {
		available = recoveryScanCap04360
	}
	if available == 0 {
		return []byte{}, true, false
	}
	fragment, err := io.ReadAll(io.NewSectionReader(source, start, available))
	return fragment, err == nil, false
}

func readCompactedRecoveryTail04360(source *os.File, size int64) ([]byte, bool) {
	if size <= 0 {
		return nil, true
	}
	startOffset := size - recoveryScanCap04360
	if startOffset < 0 {
		startOffset = 0
	}
	body, err := io.ReadAll(io.NewSectionReader(source, startOffset, size-startOffset))
	if err != nil || !bytes.HasSuffix(body, []byte("\n")) {
		return nil, false
	}
	start, cursor := 0, 0
	for cursor < len(body) {
		relativeEnd := bytes.IndexByte(body[cursor:], '\n')
		if relativeEnd < 0 {
			break
		}
		end := cursor + relativeEnd + 1
		var record map[string]any
		line := body[cursor : end-1]
		if len(line) <= recoveryLineCap04360 && json.Unmarshal(line, &record) == nil &&
			recoveryCompactionBoundary04360(record) {
			start = end
		}
		cursor = end
	}
	if start == 0 {
		return []byte{}, true
	}
	return body[start:], true
}

func recoveryCompactionBoundary04360(record map[string]any) bool {
	typ, _ := record["type"].(string)
	if typ == "compacted" || typ == "context_compacted" {
		return true
	}
	if typ != "event_msg" {
		return false
	}
	payload, _ := record["payload"].(map[string]any)
	payloadType, _ := payload["type"].(string)
	return payloadType == "compacted" || payloadType == "context_compacted"
}

func readCompressedRecoverySince04360(checkpoint recoveryCheckpoint04360, consumed int64) ([]byte, bool) {
	if consumed < 0 {
		return nil, false
	}
	before, err := os.Lstat(checkpoint.path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > recoveryCompressedPhysicalCap04360 {
		return nil, false
	}
	source, err := os.Open(checkpoint.path)
	if err != nil {
		return nil, false
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, false
	}
	decoder, err := zstd.NewReader(source,
		zstd.WithDecoderMaxMemory(32<<20),
		zstd.WithDecoderConcurrency(1),
	)
	if err != nil {
		return nil, false
	}
	defer decoder.Close()
	baseline := make([]byte, checkpoint.offset)
	if _, err = io.ReadFull(decoder, baseline); err != nil || sha256.Sum256(baseline) != checkpoint.baselineSHA {
		return nil, false
	}
	if consumed > 0 {
		if _, err = io.CopyN(io.Discard, decoder, consumed); err != nil {
			return nil, false
		}
	}
	fragment, err := io.ReadAll(io.LimitReader(decoder, recoveryScanCap04360))
	return fragment, err == nil
}

func scanRecoveryClassSince04360(
	ctx context.Context, sessionID string, fragment []byte, accepted bool,
) (string, int64, bool) {
	var recovery string
	var consumed int64
	for consumed < int64(len(fragment)) {
		if ctx.Err() != nil {
			return "", consumed, accepted
		}
		relativeEnd := bytes.IndexByte(fragment[consumed:], '\n')
		if relativeEnd < 0 {
			break
		}
		end := consumed + int64(relativeEnd)
		line := fragment[consumed:end]
		consumed = end + 1
		if len(line) == 0 {
			continue
		}
		if len(line) > recoveryLineCap04360 {
			accepted = false
			continue
		}
		var value map[string]any
		if json.Unmarshal(line, &value) != nil || !recordSession04360(value, sessionID) {
			continue
		}
		switch liveRecoveryObservationKind04360(value) {
		case recoveryObservationActivity04360:
			recovery = ""
		case recoveryObservationTerminal04360:
			class := structuredWorkflowRecoveryClass04360(value)
			if accepted && class != "" {
				recovery = class
			} else {
				recovery = ""
			}
		}
		if recordUserAccepted04360(value) {
			accepted = true
		}
		if recordTurnBoundary04360(value) {
			accepted = false
		}
	}
	return recovery, consumed, accepted
}
