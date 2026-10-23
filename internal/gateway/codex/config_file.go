package codex

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const maxCodexConfigBytes = 1 << 20

func readCodexConfig(ctx context.Context, home string) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) || filepath.Clean(home) == filepath.Dir(filepath.Clean(home)) {
		return nil, errors.New("codex profile home must be an absolute directory")
	}
	homeInfo, err := os.Lstat(home)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("codex profile home must be a real directory")
	}

	path := filepath.Join(home, "config.toml")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCodexConfigBytes {
		return nil, errors.New("codex config.toml must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open codex config.toml")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxCodexConfigBytes+1))
	if err != nil || len(content) > maxCodexConfigBytes {
		return nil, errors.New("read codex config.toml")
	}
	var config map[string]any
	if err := toml.Unmarshal(content, &config); err != nil {
		return nil, errors.New("parse codex config.toml")
	}
	return config, nil
}
