package superexpose

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	outputSourceProbeBytes = 64 * 1024
	outputSkipMaxBytes     = 4 * 1024 * 1024
)

func (service *existingSessionService) outputSource(target resolvedSessionTarget) (string, error) {
	stored, err := service.queue.rolloutPath(target.stateDB, target.threadID)
	if err != nil {
		return "", err
	}
	roots := []string{target.environment.codexHome, target.environment.codexSQLiteHome}
	if stored != "" {
		if path := validRolloutPathInRoots(stored, roots, target.threadID); path != "" {
			return path, nil
		}
		if details, inspectErr := service.process.inspect(target.writer.pid); inspectErr == nil && details != nil {
			if path := validRolloutPathInOpenFiles(stored, roots, details.openFiles, target.threadID); path != "" {
				return path, nil
			}
		}
		if rolloutPathExistsInRoots(stored, roots) {
			return "", sessionOutputSourceChanged
		}
		return "", sessionOutputSourceUnavailable
	}
	candidates := make([]string, 0, 2)
	seenRoots := map[string]bool{}
	for _, root := range roots {
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", sessionOutputSourceUnavailable
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return "", sessionOutputSourceUnavailable
		}
		if seenRoots[canonical] {
			continue
		}
		seenRoots[canonical] = true
		for _, directory := range []string{
			filepath.Join(canonical, "sessions"),
			filepath.Join(canonical, "archived_sessions"),
		} {
			if err := collectExactRollouts(directory, target.threadID, &candidates, 0); err != nil {
				return "", err
			}
		}
	}
	sort.Strings(candidates)
	candidates = compactStrings(candidates)
	switch len(candidates) {
	case 0:
		return "", sessionOutputSourceUnavailable
	case 1:
		return candidates[0], nil
	default:
		return "", sessionOutputSourceAmbiguous
	}
}

func validRolloutPathInRoots(stored string, roots []string, threadID string) string {
	for _, candidate := range rolloutCandidates(stored, roots) {
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil || !pathWithinAnyRoot(canonical, roots) ||
			!uncompressedRolloutFile(filepath.Base(canonical)) ||
			legacyThreadID(canonical) != threadID {
			continue
		}
		return filepath.Clean(canonical)
	}
	return ""
}

func validRolloutPathInOpenFiles(stored string, roots []string, files []openProcessFile, threadID string) string {
	for _, candidate := range rolloutCandidates(stored, roots) {
		if containsParentTraversal(candidate) || !pathWithinAnyRoot(candidate, roots) {
			continue
		}
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		for _, file := range files {
			openCanonical, err := filepath.EvalSymlinks(file.path)
			if err != nil || filepath.Clean(openCanonical) != filepath.Clean(canonical) {
				continue
			}
			if uncompressedRolloutFile(filepath.Base(openCanonical)) &&
				legacyThreadID(openCanonical) == threadID {
				if stat, err := os.Stat(openCanonical); err == nil && stat.Mode().IsRegular() {
					return filepath.Clean(openCanonical)
				}
			}
		}
	}
	return ""
}

func rolloutCandidates(stored string, roots []string) []string {
	if filepath.IsAbs(stored) {
		return []string{stored}
	}
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		result = append(result, filepath.Join(root, stored))
	}
	return result
}

func rolloutPathExistsInRoots(stored string, roots []string) bool {
	for _, candidate := range rolloutCandidates(stored, roots) {
		if _, err := os.Lstat(candidate); err == nil {
			return true
		}
	}
	return false
}

func collectExactRollouts(root, threadID string, output *[]string, depth int) error {
	if depth > 8 {
		return sessionOutputReadFailed
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			if err := collectExactRollouts(path, threadID, output, depth+1); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() && uncompressedRolloutFile(entry.Name()) &&
			legacyThreadID(path) == threadID {
			canonical, err := filepath.Abs(path)
			if err == nil {
				*output = append(*output, filepath.Clean(canonical))
			}
		}
		if len(*output) > 2 {
			return sessionOutputSourceAmbiguous
		}
	}
	return nil
}

func uncompressedRolloutFile(name string) bool {
	return strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl")
}

func pathWithinAnyRoot(path string, roots []string) bool {
	path, err := canonicalContainmentPath(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		root, err := canonicalContainmentPath(root)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
			!filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func canonicalContainmentPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute), nil
}

func containsParentTraversal(path string) bool {
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

func outputSourceID(path, threadID string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", sessionOutputSourceUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return "", sessionOutputSourceUnavailable
	}
	defer file.Close()
	identity, err := openedFileIdentity(path, file, info)
	if err != nil {
		return "", sessionOutputSourceChanged
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(threadID))
	_, _ = hash.Write([]byte(filepath.Clean(path)))
	_, _ = hash.Write(identity)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sourceCheckpointID(path string, offset uint64) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", sessionOutputSourceChanged
	}
	if uint64(info.Size()) < offset {
		return "", sessionOutputSourceChanged
	}
	file, err := os.Open(path)
	if err != nil {
		return "", sessionOutputSourceChanged
	}
	defer file.Close()
	if _, err := openedFileIdentity(path, file, info); err != nil {
		return "", sessionOutputSourceChanged
	}
	prefixLen := min(offset, uint64(outputSourceProbeBytes))
	prefix := make([]byte, prefixLen)
	if _, err := io.ReadFull(file, prefix); err != nil {
		return "", sessionOutputSourceChanged
	}
	hash := sha256.New()
	var offsetBytes [8]byte
	for index := range 8 {
		offsetBytes[index] = byte(offset >> (8 * index))
	}
	_, _ = hash.Write(offsetBytes[:])
	_, _ = hash.Write(prefix)
	if offset > 0 {
		start := offset
		if start > 4096 {
			start -= 4096
		} else {
			start = 0
		}
		if _, err := file.Seek(int64(start), io.SeekStart); err != nil {
			return "", sessionOutputSourceChanged
		}
		window := make([]byte, offset-start)
		if _, err := io.ReadFull(file, window); err != nil {
			return "", sessionOutputSourceChanged
		}
		_, _ = hash.Write(window)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
