package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func persistSessionFileAttachments(codexHome, sessionFile string) (string, bool, error) {
	contents, ok, err := readSessionAttachmentFile(sessionFile)
	if err != nil {
		return "", false, normalizeSessionAttachmentReadError(err)
	}
	if !ok {
		return "", false, nil
	}
	rewritten, err := rewriteSessionPersistedAttachmentPaths(codexHome, contents)
	if err != nil {
		return "", false, err
	}
	if int64(len(rewritten)) > sessionAttachmentRewriteMaxBytes {
		return "", false, nil
	}
	if rewritten != contents {
		if err := writeSessionAttachmentFile(sessionFile, rewritten); err != nil {
			return "", false, err
		}
	}
	return rewritten, true, nil
}

func normalizeSessionAttachmentReadError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "symlink"):
		return errors.New("refusing to read codex session file through symlink")
	case strings.Contains(message, "not a file"):
		return errors.New("codex session path is not a file")
	case strings.Contains(message, "changed while opening"):
		return errors.New("codex session file changed while opening")
	case strings.Contains(message, "utf-8"), strings.Contains(message, "decode session attachment"):
		return errors.New("failed to decode codex session file")
	default:
		return errors.New("failed to read codex session file")
	}
}

func writeSessionAttachmentFile(path, contents string) error {
	if int64(len(contents)) > sessionAttachmentRewriteMaxBytes {
		return fmt.Errorf("rewritten codex session exceeds safe size limit (%d bytes)", sessionAttachmentRewriteMaxBytes)
	}
	info, err := os.Stat(path)
	if err != nil {
		return errors.New("failed to read codex session file permissions")
	}
	tempPath := sessionAttachmentTempPath(path)
	_ = os.Remove(tempPath)

	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return errors.New("failed to write codex session file")
	}
	keepTemp := true
	defer func() {
		_ = file.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	encoded, err := encodeSessionAttachmentContents(path, contents)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		return errors.New("failed to write codex session file")
	}
	if err := file.Sync(); err != nil {
		return errors.New("failed to sync codex session file")
	}
	if err := file.Close(); err != nil {
		return errors.New("failed to sync codex session file")
	}
	if err := os.Rename(tempPath, path); err != nil {
		return errors.New("failed to write codex session file")
	}
	keepTemp = false
	if err := syncSessionAttachmentDirectory(filepath.Dir(path)); err != nil {
		return errors.New("failed to sync codex session directory")
	}
	return nil
}

func encodeSessionAttachmentContents(path, contents string) ([]byte, error) {
	if !isCompressedSessionFile(path) {
		return []byte(contents), nil
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	if err != nil {
		return nil, errors.New("failed to encode codex session file")
	}
	defer encoder.Close()
	return encoder.EncodeAll([]byte(contents), nil), nil
}

func sessionAttachmentTempPath(path string) string {
	extension := strings.TrimPrefix(filepath.Ext(path), ".")
	if extension == "" {
		extension = "jsonl"
		path += ".jsonl"
	}
	return path + ".prodex-attachments-tmp-" + strconv.Itoa(os.Getpid())
}
