package codex

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

const sessionAttachmentRewriteMaxBytes int64 = 64 * 1024 * 1024

func readSessionAttachmentFile(path string) (string, bool, error) {
	info, err := inspectSessionRegularFile(path)
	if err != nil {
		return "", false, err
	}
	if info.Size() > sessionAttachmentRewriteMaxBytes {
		return "", false, nil
	}
	file, err := openSessionRegularFileFromMetadata(path, info)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	var reader io.Reader = file
	var decoder *zstd.Decoder
	if isCompressedSessionFile(path) {
		decoder, err = zstd.NewReader(file)
		if err != nil {
			return "", false, fmt.Errorf("decode compressed session file: %w", err)
		}
		defer decoder.Close()
		reader = decoder
	}
	content, err := io.ReadAll(io.LimitReader(reader, sessionAttachmentRewriteMaxBytes+1))
	if err != nil {
		return "", false, fmt.Errorf("read session attachment file: %w", err)
	}
	if int64(len(content)) > sessionAttachmentRewriteMaxBytes {
		return "", false, nil
	}
	if !utf8.Valid(content) {
		return "", false, errors.New("decode session attachment file as UTF-8")
	}
	return string(content), true, nil
}

func isCompressedSessionFile(path string) bool {
	return strings.HasSuffix(path, ".jsonl.zst")
}
