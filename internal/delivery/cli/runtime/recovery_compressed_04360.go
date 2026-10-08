package runtime

import (
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

const recoveryCompressedBaselineCap04360 int64 = 8 << 20
const recoveryCompressedPhysicalCap04360 int64 = 16 << 20

// Decode only a bounded, regular Codex rollout. Prodex can rewrite a
// compressed session atomically; callers validate an immutable decoded
// prefix hash instead of relying on the underlying inode to stay equal.
func readCompressedRecovery04360(path string, maxOutput int64) ([]byte, bool) {
	if maxOutput <= 0 || maxOutput > recoveryCompressedBaselineCap04360+recoveryScanCap04360 {
		return nil, false
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > recoveryCompressedPhysicalCap04360 {
		return nil, false
	}
	source, err := os.Open(path)
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
	payload, err := io.ReadAll(io.LimitReader(decoder, maxOutput+1))
	if err != nil || int64(len(payload)) > maxOutput {
		return nil, false
	}
	return payload, true
}
