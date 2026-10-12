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
func (checkpoint recoveryCheckpoint04360) scanAcceptedRecovery04360(
	ctx context.Context, sessionID string, consumed int64, accepted bool,
) (string, int64, bool, bool) {
	if !checkpoint.valid || ctx.Err() != nil || !sessionentity.ValidID(sessionID) || consumed < 0 {
		return "", consumed, accepted, false
	}
	fragment, ok := checkpoint.readAppendedRecovery04360(ctx, consumed)
	if !ok {
		return "", consumed, accepted, false
	}
	class, advanced, accepted := scanRecoveryClassSince04360(ctx, sessionID, fragment, accepted)
	return class, consumed + advanced, accepted, true
}

func (checkpoint recoveryCheckpoint04360) readAppendedRecovery04360(
	ctx context.Context, consumed int64,
) ([]byte, bool) {
	if ctx.Err() != nil || consumed < 0 {
		return nil, false
	}
	if checkpoint.compressed {
		return readCompressedRecoverySince04360(checkpoint, consumed)
	}
	source, err := os.Open(checkpoint.path)
	if err != nil {
		return nil, false
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(checkpoint.origin, info) {
		return nil, false
	}
	size := info.Size()
	start := checkpoint.offset + consumed
	if start < checkpoint.offset || start > size {
		return nil, false
	}
	baselineSHA, ok := recoveryPrefixHash04360(source, checkpoint.offset)
	if !ok || baselineSHA != checkpoint.baselineSHA {
		return nil, false
	}
	available := size - start
	if available > recoveryScanCap04360 {
		available = recoveryScanCap04360
	}
	if available == 0 {
		return []byte{}, true
	}
	fragment, err := io.ReadAll(io.NewSectionReader(source, start, available))
	return fragment, err == nil
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
