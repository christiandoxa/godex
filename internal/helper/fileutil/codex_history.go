package fileutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxCodexHistoryMergeBytes = 64 << 20

type codexHistoryLine struct {
	text  string
	ts    int64
	hasTS bool
	order int
}

// MergeCodexHistory merges one profile history into an existing shared history.
// Exact duplicate lines keep their first occurrence; timestamped JSON lines sort
// by ts while unparseable/untimestamped lines preserve encounter order.
func MergeCodexHistory(source, destination string) error {
	destinationInfo, lines, err := loadCodexHistory(destination, nil)
	if err != nil {
		return err
	}
	sourceInfo, lines, err := loadCodexHistory(source, lines)
	if err != nil {
		return err
	}
	_ = sourceInfo

	seen := make(map[string]struct{}, len(lines))
	unique := lines[:0]
	total := 0
	for _, line := range lines {
		if _, exists := seen[line.text]; exists {
			continue
		}
		seen[line.text] = struct{}{}
		if len(unique) > 0 {
			total++
		}
		total += len(line.text)
		if total > maxCodexHistoryMergeBytes {
			return fmt.Errorf("merged history %s exceeds safe size limit (%d bytes)", destination, maxCodexHistoryMergeBytes)
		}
		unique = append(unique, line)
	}
	sort.SliceStable(unique, func(i, j int) bool {
		left, right := unique[i], unique[j]
		if left.hasTS && right.hasTS && left.ts != right.ts {
			return left.ts < right.ts
		}
		return left.order < right.order
	})

	var content strings.Builder
	content.Grow(total)
	for index, line := range unique {
		if index > 0 {
			content.WriteByte('\n')
		}
		content.WriteString(line.text)
	}
	return replaceCodexHistory(destination, []byte(content.String()), destinationInfo.Mode().Perm())
}

func loadCodexHistory(path string, lines []codexHistoryLine) (os.FileInfo, []codexHistoryLine, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, lines, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, lines, fmt.Errorf("history path %s is not a real regular file", path)
	}
	if info.Size() > maxCodexHistoryMergeBytes {
		return nil, lines, fmt.Errorf("history %s exceeds safe size limit (%d bytes)", path, maxCodexHistoryMergeBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, lines, err
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, lines, statErr
	}
	current, statErr := os.Lstat(path)
	if statErr != nil {
		_ = file.Close()
		return nil, lines, statErr
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, opened) || !os.SameFile(info, current) {
		_ = file.Close()
		return nil, lines, fmt.Errorf("history path changed while opening %s", path)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxCodexHistoryMergeBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, lines, err
	}
	if len(content) > maxCodexHistoryMergeBytes {
		return nil, lines, fmt.Errorf("history %s exceeds safe size limit (%d bytes)", path, maxCodexHistoryMergeBytes)
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" {
			continue
		}
		entry := codexHistoryLine{text: line, order: len(lines)}
		var value struct {
			TS *int64 `json:"ts"`
		}
		if json.Unmarshal([]byte(line), &value) == nil && value.TS != nil {
			entry.ts, entry.hasTS = *value.TS, true
		}
		lines = append(lines, entry)
	}
	return info, lines, nil
}

func replaceCodexHistory(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".codex-history-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	writeErr := temporary.Chmod(mode)
	if writeErr == nil {
		_, writeErr = temporary.Write(content)
	}
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	writeErr = errors.Join(writeErr, temporary.Close())
	if writeErr != nil {
		return writeErr
	}
	if err := Replace(temporaryPath, path); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(path))
}
