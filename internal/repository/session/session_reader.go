package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/klauspost/compress/zstd"
)

const (
	maxSessionFiles     = 4096
	maxSessionScanBytes = 4 << 20
	maxSessionLineBytes = 512 << 10
)

type Reader struct{}

func NewReader() *Reader { return &Reader{} }

func (*Reader) List(ctx context.Context, codexHome string) ([]sessionentity.Session, error) {
	var paths []string
	for _, directory := range []string{"sessions", "archived_sessions"} {
		found, err := collectSessionPaths(ctx, filepath.Join(codexHome, directory), maxSessionFiles-len(paths))
		if err != nil {
			return nil, err
		}
		paths = append(paths, found...)
		if len(paths) > maxSessionFiles {
			return nil, fmt.Errorf("codex session count exceeds safe limit of %d", maxSessionFiles)
		}
	}

	reports := make([]sessionentity.Session, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		report, ok, err := readSessionReport(ctx, path)
		if err != nil {
			return nil, err
		}
		if ok {
			reports = append(reports, report)
		}
	}
	if err := applySessionIndex(ctx, codexHome, reports); err != nil {
		return nil, err
	}
	return reports, nil
}

func collectSessionPaths(ctx context.Context, root string, remaining int) ([]string, error) {
	if err := validateSessionRoot(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		include, err := sessionWalkEntry(ctx, root, entry, walkErr)
		if err != nil {
			return err
		}
		if entry != nil && entry.Type()&os.ModeSymlink != 0 && entry.IsDir() {
			return filepath.SkipDir
		}
		if include {
			paths = append(paths, path)
			if len(paths) > remaining {
				return fmt.Errorf("codex session count exceeds safe limit of %d", maxSessionFiles)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Codex sessions: %w", err)
	}
	return paths, nil
}

func validateSessionRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return fmt.Errorf("inspect Codex sessions: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("codex session root %s must be a real directory", root)
	}
	return nil
}

func sessionWalkEntry(ctx context.Context, root string, entry os.DirEntry, walkErr error) (bool, error) {
	if walkErr != nil {
		return false, walkErr
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if entry == nil || entry.Name() == filepath.Base(root) || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
		return false, nil
	}
	if !sessionFileName(entry.Name()) {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func sessionFileName(name string) bool {
	return strings.HasPrefix(name, "rollout-") && (strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonl.zst"))
}

func readSessionReport(ctx context.Context, path string) (sessionentity.Session, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sessionentity.Session{}, false, fmt.Errorf("inspect Codex session: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return sessionentity.Session{}, false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return sessionentity.Session{}, false, fmt.Errorf("open Codex session: %w", err)
	}
	defer file.Close()

	report := sessionentity.Session{
		Path:        path,
		UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339),
		UpdatedUnix: info.ModTime().Unix(),
	}
	var reader io.Reader = file
	if strings.HasSuffix(path, ".jsonl.zst") {
		decoder, decodeErr := zstd.NewReader(file, zstd.WithDecoderMaxMemory(8<<20))
		if decodeErr != nil {
			return sessionentity.Session{}, false, fmt.Errorf("decode compressed Codex session: %w", decodeErr)
		}
		defer decoder.Close()
		reader = decoder
	}
	scanner := bufio.NewScanner(io.LimitReader(reader, maxSessionScanBytes))
	scanner.Buffer(make([]byte, 64<<10), maxSessionLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return sessionentity.Session{}, false, err
		}
		applySessionMetadata(&report, scanner.Bytes())
	}
	if err := scanner.Err(); err != nil && report.ID == "" {
		return sessionentity.Session{}, false, fmt.Errorf("scan Codex session metadata: %w", err)
	}
	if !sessionentity.ValidID(report.ID) {
		return sessionentity.Session{}, false, nil
	}
	return report, true, nil
}
