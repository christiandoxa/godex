package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const sessionRepairLockFile = ".prodex-session-repair.lock"

var sessionRepairTempSequence atomic.Uint64

type sessionRepairRevision struct {
	length   int64
	modified int64
	mode     os.FileMode
	platform [3]int64
}

type sessionRepairTransaction struct {
	path        string
	parent      string
	source      *os.File
	revision    sessionRepairRevision
	sourceBytes []byte
	contents    string
	releaseLock func() error
}

func beginSessionRepairTransaction(path string) (*sessionRepairTransaction, error) {
	parent := filepath.Dir(path)
	if err := requireSessionRepairDirectory(parent); err != nil {
		return nil, err
	}
	root := sessionRepairRepositoryRoot(path)
	if err := requireSessionRepairDirectory(root); err != nil {
		return nil, err
	}
	release, err := lockfile.Acquire(context.Background(), filepath.Join(root, sessionRepairLockFile))
	if err != nil {
		return nil, fmt.Errorf("failed to lock session repository %s: %w", root, err)
	}
	lockPath := filepath.Join(root, sessionRepairLockFile)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(lockPath, 0o600); err != nil {
			_ = release()
			return nil, fmt.Errorf("failed to make repair lock private %s: %w", lockPath, err)
		}
	}

	source, info, err := openSessionRegularFileNoFollow(path)
	if err != nil {
		_ = release()
		return nil, err
	}
	if info.Size() > sessionAttachmentRewriteMaxBytes {
		_ = source.Close()
		_ = release()
		return nil, fmt.Errorf("session %s exceeds safe size limit (%d bytes)", path, sessionAttachmentRewriteMaxBytes)
	}
	raw, err := readSessionRepairRaw(source)
	if err != nil {
		_ = source.Close()
		_ = release()
		return nil, fmt.Errorf("failed to read session %s: %w", path, err)
	}
	decoded, err := decodeSessionRepairBytes(path, raw)
	if err != nil {
		_ = source.Close()
		_ = release()
		return nil, err
	}
	if int64(len(decoded)) > sessionAttachmentRewriteMaxBytes {
		_ = source.Close()
		_ = release()
		return nil, fmt.Errorf("session %s exceeds safe size limit (%d bytes)", path, sessionAttachmentRewriteMaxBytes)
	}
	if !utf8.Valid(decoded) {
		_ = source.Close()
		_ = release()
		return nil, fmt.Errorf("failed to decode session %s", path)
	}
	transaction := &sessionRepairTransaction{
		path: path, parent: parent, source: source,
		revision:    sessionRepairRevisionFromInfo(info),
		sourceBytes: raw, contents: string(decoded), releaseLock: release,
	}
	if err := transaction.verifySource(); err != nil {
		transaction.close()
		return nil, err
	}
	return transaction, nil
}

func (transaction *sessionRepairTransaction) close() {
	if transaction.source != nil {
		_ = transaction.source.Close()
		transaction.source = nil
	}
	if transaction.releaseLock != nil {
		_ = transaction.releaseLock()
		transaction.releaseLock = nil
	}
}

func (transaction *sessionRepairTransaction) commit(repaired []byte, beforeVerify func()) (err error) {
	if err := transaction.verifySource(); err != nil {
		return err
	}
	backupPath, backupInfo, backupCreated, err := ensureSessionRepairBackup(transaction.path, transaction.sourceBytes)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if success || !backupCreated {
			return
		}
		removeSessionRepairOwnedFile(backupPath, backupInfo)
		_ = syncSessionAttachmentDirectory(transaction.parent)
	}()

	tempFile, tempPath, tempInfo, err := createSessionRepairTemporary(transaction.path)
	if err != nil {
		return err
	}
	tempArmed := true
	defer func() {
		_ = tempFile.Close()
		if tempArmed {
			removeSessionRepairOwnedFile(tempPath, tempInfo)
		}
	}()

	encoded, err := encodeSessionRepairBytes(transaction.path, repaired)
	if err != nil {
		return err
	}
	if _, err := tempFile.Write(encoded); err != nil {
		return fmt.Errorf("failed to write repaired session %s: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync repaired session %s: %w", tempPath, err)
	}
	if err := syncSessionAttachmentDirectory(transaction.parent); err != nil {
		return fmt.Errorf("failed to sync directory %s: %w", transaction.parent, err)
	}

	if beforeVerify != nil {
		beforeVerify()
	}
	if err := transaction.verifySource(); err != nil {
		return err
	}
	if err := transaction.source.Close(); err != nil {
		return fmt.Errorf("failed to close session %s: %w", transaction.path, err)
	}
	transaction.source = nil
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close repaired session %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, transaction.path); err != nil {
		return fmt.Errorf("failed to replace repaired session %s: %w", transaction.path, err)
	}
	tempArmed = false
	current, err := os.Lstat(transaction.path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(tempInfo, current) {
		if err != nil {
			return fmt.Errorf("failed to verify repaired session %s: %w", transaction.path, err)
		}
		return fmt.Errorf("repaired session replacement identity mismatch: %s", transaction.path)
	}
	if err := syncSessionAttachmentDirectory(transaction.parent); err != nil {
		return fmt.Errorf("failed to sync directory %s: %w", transaction.parent, err)
	}
	success = true
	return nil
}

func (transaction *sessionRepairTransaction) verifySource() error {
	if transaction.source == nil {
		return errors.New("session repair source is closed")
	}
	opened, err := transaction.source.Stat()
	if err != nil || sessionRepairRevisionFromInfo(opened) != transaction.revision {
		return fmt.Errorf("session changed during repair: %s", transaction.path)
	}
	named, err := os.Lstat(transaction.path)
	if err != nil || named.Mode()&os.ModeSymlink != 0 || !named.Mode().IsRegular() ||
		sessionRepairRevisionFromInfo(named) != transaction.revision {
		return fmt.Errorf("session changed during repair: %s", transaction.path)
	}
	matches, err := sessionOpenedFileMatchesPath(named, transaction.path, transaction.source)
	if err != nil || !matches {
		return fmt.Errorf("session changed during repair: %s", transaction.path)
	}
	if _, err := transaction.source.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("session changed during repair: %s", transaction.path)
	}
	current, err := readSessionRepairRaw(transaction.source)
	if err != nil || !bytes.Equal(current, transaction.sourceBytes) {
		return fmt.Errorf("session changed during repair: %s", transaction.path)
	}
	return nil
}

func sessionRepairRevisionFromInfo(info os.FileInfo) sessionRepairRevision {
	return sessionRepairRevision{
		length: info.Size(), modified: info.ModTime().UnixNano(), mode: info.Mode(),
		platform: sessionRepairPlatformRevision(info),
	}
}

func readSessionRepairRaw(file *os.File) ([]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	bytes, err := io.ReadAll(io.LimitReader(file, sessionAttachmentRewriteMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bytes)) > sessionAttachmentRewriteMaxBytes {
		return nil, fmt.Errorf("session exceeds safe size limit (%d bytes)", sessionAttachmentRewriteMaxBytes)
	}
	return bytes, nil
}

func decodeSessionRepairBytes(path string, raw []byte) ([]byte, error) {
	if !isCompressedSessionFile(path) {
		return append([]byte(nil), raw...), nil
	}
	decoder, err := zstd.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress session: %w", err)
	}
	defer decoder.Close()
	decoded, err := io.ReadAll(io.LimitReader(decoder, sessionAttachmentRewriteMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress session: %w", err)
	}
	return decoded, nil
}

func encodeSessionRepairBytes(path string, repaired []byte) ([]byte, error) {
	if !isCompressedSessionFile(path) {
		return append([]byte(nil), repaired...), nil
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	if err != nil {
		return nil, fmt.Errorf("failed to compress repaired session: %w", err)
	}
	defer encoder.Close()
	return encoder.EncodeAll(repaired, nil), nil
}

func sessionRepairRepositoryRoot(path string) string {
	fallback := filepath.Dir(path)
	for current := fallback; ; current = filepath.Dir(current) {
		base := filepath.Base(current)
		if base == "sessions" || base == "archived_sessions" {
			return filepath.Dir(current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fallback
		}
	}
}

func requireSessionRepairDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to inspect directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("repair directory %s is not a regular directory", path)
	}
	return nil
}

func ensureSessionRepairBackup(path string, source []byte) (string, os.FileInfo, bool, error) {
	backupPath := sessionRepairBackupPath(path)
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(source); writeErr != nil {
			_ = file.Close()
			_ = os.Remove(backupPath)
			return "", nil, false, fmt.Errorf("failed to backup session %s: %w", path, writeErr)
		}
		if syncErr := file.Sync(); syncErr != nil {
			_ = file.Close()
			_ = os.Remove(backupPath)
			return "", nil, false, fmt.Errorf("failed to sync backup %s: %w", backupPath, syncErr)
		}
		info, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil {
			_ = os.Remove(backupPath)
			return "", nil, false, statErr
		}
		if closeErr != nil {
			_ = os.Remove(backupPath)
			return "", nil, false, closeErr
		}
		if dirErr := syncSessionAttachmentDirectory(filepath.Dir(path)); dirErr != nil {
			_ = os.Remove(backupPath)
			return "", nil, false, dirErr
		}
		return backupPath, info, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return "", nil, false, fmt.Errorf("failed to create backup %s: %w", backupPath, err)
	}
	existing, info, openErr := openSessionRegularFileNoFollow(backupPath)
	if openErr != nil {
		return "", nil, false, fmt.Errorf("repair backup is not a regular file: %s: %w", backupPath, openErr)
	}
	defer existing.Close()
	if runtime.GOOS != "windows" {
		if chmodErr := existing.Chmod(0o600); chmodErr != nil {
			return "", nil, false, chmodErr
		}
	}
	current, statErr := os.Lstat(backupPath)
	if statErr != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() {
		return "", nil, false, fmt.Errorf("repair backup is not a regular file: %s", backupPath)
	}
	matches, matchErr := sessionOpenedFileMatchesPath(current, backupPath, existing)
	if matchErr != nil || !matches {
		return "", nil, false, fmt.Errorf("repair backup changed while in use: %s", backupPath)
	}
	if runtime.GOOS != "windows" {
		if syncErr := existing.Sync(); syncErr != nil {
			return "", nil, false, syncErr
		}
	}
	return backupPath, info, false, nil
}

func sessionRepairBackupPath(path string) string {
	if filepath.Ext(path) == "" {
		return path + ".session.prodex-repair-bak"
	}
	return path + ".prodex-repair-bak"
}

func createSessionRepairTemporary(path string) (*os.File, string, os.FileInfo, error) {
	parent := filepath.Dir(path)
	base := filepath.Base(path)
	var last error
	for attempt := 0; attempt < 64; attempt++ {
		sequence := sessionRepairTempSequence.Add(1) - 1
		tempPath := filepath.Join(parent, "."+base+".prodex-repair-tmp-"+strconv.Itoa(os.Getpid())+"-"+strconv.FormatUint(sequence, 10))
		file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			last = err
			continue
		}
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to create repaired session temporary file for %s: %w", path, err)
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			_ = os.Remove(tempPath)
			return nil, "", nil, statErr
		}
		return file, tempPath, info, nil
	}
	if last == nil {
		last = os.ErrExist
	}
	return nil, "", nil, fmt.Errorf("failed to create repaired session temporary file for %s: %w", path, last)
}

func removeSessionRepairOwnedFile(path string, expected os.FileInfo) {
	if expected == nil {
		return
	}
	current, err := os.Lstat(path)
	if err == nil && current.Mode().IsRegular() && os.SameFile(expected, current) {
		_ = os.Remove(path)
	}
}
