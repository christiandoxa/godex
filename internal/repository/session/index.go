package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// Codex keeps thread renames in an append-only index, separate from rollouts.
func applySessionIndex(ctx context.Context, home string, reports []sessionentity.Session) error {
	path := filepath.Join(home, "session_index.jsonl")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect codex session index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("codex session index must be a regular file")
	}
	const maxIndexBytes = 16 << 20
	if info.Size() > maxIndexBytes {
		return errors.New("codex session index exceeds safe size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open codex session index: %w", err)
	}
	defer file.Close()
	byID := make(map[string][]int, len(reports))
	for i := range reports {
		byID[reports[i].ID] = append(byID[reports[i].ID], i)
	}
	scanner := bufio.NewScanner(io.LimitReader(file, maxIndexBytes+1))
	scanner.Buffer(make([]byte, 64<<10), maxSessionLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		for _, i := range byID[entry.ID] {
			reports[i].ThreadName = entry.ThreadName
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.New("scan codex session index")
	}
	return nil
}
