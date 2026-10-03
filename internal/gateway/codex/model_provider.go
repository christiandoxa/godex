package codex

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	"github.com/pelletier/go-toml/v2"
)

const maxCodexConfigBytes = 1 << 20

func (*CodexProcess) InspectModelProvider(ctx context.Context, home string) (*profilemodel.ModelProviderSetting, error) {
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
	providerID, ok := config["model_provider"].(string)
	if !ok || strings.TrimSpace(providerID) == "" || strings.EqualFold(providerID, "openai") {
		return nil, nil
	}
	return &profilemodel.ModelProviderSetting{ProviderID: providerID, Source: "config.toml"}, nil
}
