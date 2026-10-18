package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/klauspost/compress/zstd"
)

const maxWorkspaceSessionFiles = 65_536

func (*Reader) Workspaces(ctx context.Context, home string) ([]sessionentity.Workspace, error) {
	var paths []string
	visited := 0
	for _, directory := range []string{"sessions", "archived_sessions"} {
		if err := collectWorkspacePaths(ctx, filepath.Join(home, directory), &visited, &paths); err != nil {
			return nil, err
		}
	}
	var candidates []sessionentity.Workspace
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		workspace, err := initialWorkspace(ctx, path)
		if err == nil && workspace != "" {
			candidates = append(candidates, sessionentity.Workspace{Path: workspace, ModifiedUnix: max(0, info.ModTime().Unix())})
		}
	}
	return candidates, ctx.Err()
}

// ReadDir on an open directory retains native discovery order for mtime ties.
func collectWorkspacePaths(ctx context.Context, root string, visited *int, paths *[]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSessionRoot(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	directory, err := os.Open(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	for {
		entries, readErr := directory.ReadDir(256)
		for _, entry := range entries {
			*visited += 1
			if *visited > maxWorkspaceSessionFiles {
				return fmt.Errorf("recent workspace scan exceeds %d entries", maxWorkspaceSessionFiles)
			}
			path := filepath.Join(root, entry.Name())
			if entry.IsDir() {
				if err := collectWorkspacePaths(ctx, path, visited, paths); err != nil {
					return err
				}
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".jsonl.zst") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				*paths = append(*paths, path)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func initialWorkspace(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var source io.Reader = file
	if strings.HasSuffix(path, ".zst") {
		decoder, err := zstd.NewReader(file, zstd.WithDecoderMaxMemory(8<<20))
		if err != nil {
			return "", err
		}
		defer decoder.Close()
		source = decoder
	}
	source = io.LimitReader(source, maxSessionScanBytes)
	legacyJSON := strings.HasSuffix(path, ".json")
	if legacyJSON {
		content, err := io.ReadAll(source)
		if err != nil {
			return "", err
		}
		if json.Valid(content) {
			report, valid := workspaceMetadata(content)
			if valid {
				return report.CWD, nil
			}
			return "", nil
		}
		source = bytes.NewReader(content)
	}
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64<<10), maxSessionLineBytes)
	var report sessionentity.Session
	started := false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		if !started {
			var valid bool
			report, valid = workspaceMetadata(scanner.Bytes())
			if !valid {
				return "", nil
			}
			if !legacyJSON {
				return report.CWD, nil
			}
			started = true
		} else {
			applySessionMetadata(&report, scanner.Bytes())
		}
	}
	return report.CWD, scanner.Err()
}

func workspaceMetadata(content []byte) (sessionentity.Session, bool) {
	var report sessionentity.Session
	var fields map[string]json.RawMessage
	if json.Unmarshal(content, &fields) != nil {
		return report, false
	}
	var kind string
	if raw, ok := fields["type"]; ok && json.Unmarshal(raw, &kind) == nil && kind != "session_meta" && string(raw) != "null" {
		return report, false
	}
	applySessionMetadata(&report, content)
	return report, report.ID != ""
}

func (*Reader) WorkspaceAliases(ctx context.Context, workspace string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	aliases := []string{workspace, filepath.Clean(workspace)}
	if canonical, err := filepath.EvalSymlinks(workspace); err == nil {
		if runtime.GOOS == "windows" {
			if rest, ok := strings.CutPrefix(canonical, `\\?\UNC\`); ok {
				aliases = append(aliases, `\\`+rest)
			} else if rest, ok := strings.CutPrefix(canonical, `\\?\`); ok {
				aliases = append(aliases, rest)
			}
		}
		aliases = append(aliases, canonical)
	}
	return aliases, nil
}
