package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	maxLogBytes  = 8 << 20
	maxLineBytes = 256 << 10
)

type Log struct {
	root string
}

func NewLog(root string) *Log {
	return &Log{root: filepath.Clean(root)}
}

func (log *Log) Append(ctx context.Context, event runtimemodel.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := log.prepare(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, log.guardPath())
	if err != nil {
		return err
	}
	defer release()
	if err := log.rotateIfNeeded(); err != nil {
		return err
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	content = append(content, '\n')
	file, err := os.OpenFile(log.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open runtime log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (log *Log) Tail(ctx context.Context, limit int) ([]runtimemodel.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	if err := log.prepare(); err != nil {
		return nil, err
	}
	file, err := os.Open(log.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open runtime log: %w", err)
	}
	defer file.Close()
	return readTail(file, limit)
}

func readTail(reader io.Reader, limit int) ([]runtimemodel.Event, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	events := make([]runtimemodel.Event, 0, limit)
	for scanner.Scan() {
		var event runtimemodel.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if len(events) == limit {
			copy(events, events[1:])
			events[len(events)-1] = event
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (log *Log) prepare() error {
	if !filepath.IsAbs(log.root) || log.root == filepath.Dir(log.root) {
		return errors.New("invalid runtime log home")
	}
	if err := os.MkdirAll(log.root, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(log.root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime log home must be a real directory")
	}
	return os.Chmod(log.root, 0o700)
}

func (log *Log) rotateIfNeeded() error {
	info, err := os.Lstat(log.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("runtime log must be a regular file")
	}
	if info.Size() < maxLogBytes {
		return nil
	}
	_ = os.Remove(log.rotatedPath())
	return os.Rename(log.path(), log.rotatedPath())
}

func (log *Log) path() string {
	return filepath.Join(log.root, "runtime.jsonl")
}

func (log *Log) rotatedPath() string {
	return filepath.Join(log.root, "runtime.1.jsonl")
}

func (log *Log) guardPath() string {
	return filepath.Join(log.root, "runtime.guard")
}
