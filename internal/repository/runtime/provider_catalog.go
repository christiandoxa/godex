package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type ProviderCatalogStore struct{}

type providerCatalogEnvelope struct {
	Models []map[string]any `json:"models"`
}

func NewProviderCatalogStore() *ProviderCatalogStore { return &ProviderCatalogStore{} }

func (*ProviderCatalogStore) WriteCopilotRuntime(home string, models []map[string]any) (string, error) {
	if len(models) == 0 {
		return "", nil
	}
	return writeProviderCatalog(home, proxymodel.CopilotRuntimeCatalogFile, models)
}

func (*ProviderCatalogStore) ReadCopilotRuntime(home string) ([]map[string]any, error) {
	return readProviderCatalog(home, proxymodel.CopilotRuntimeCatalogFile)
}

func (*ProviderCatalogStore) ReadKiroProfile(home string) ([]map[string]any, error) {
	return readProviderCatalog(home, proxymodel.KiroProfileModelCatalogFile)
}

func (*ProviderCatalogStore) WriteExternal(home string, models []map[string]any) (string, error) {
	return writeProviderCatalog(home, proxymodel.ExternalProviderCatalogFile, models)
}

func (*ProviderCatalogStore) WriteDeepSeek(home string, models []map[string]any) (string, error) {
	return writeProviderCatalog(home, proxymodel.DeepSeekModelCatalogFile, models)
}

func writeProviderCatalog(home, name string, models []map[string]any) (string, error) {
	if len(models) == 0 || len(models) > proxymodel.ProviderCatalogMaxItems {
		return "", fmt.Errorf("provider model catalog must contain 1..=%d models", proxymodel.ProviderCatalogMaxItems)
	}
	if err := validateCatalogHome(home); err != nil {
		return "", err
	}
	path := filepath.Join(home, name)
	if err := prepareCatalogTarget(path); err != nil {
		return "", err
	}
	content, err := json.MarshalIndent(providerCatalogEnvelope{Models: models}, "", "  ")
	if err != nil {
		return "", errors.New("serialize provider model catalog")
	}
	if len(content) > proxymodel.ProviderCatalogMaxBytes {
		return "", fmt.Errorf("provider model catalog exceeds safe size limit (%d bytes)", proxymodel.ProviderCatalogMaxBytes)
	}
	content = append(content, '\n')
	if _, err := fileutil.AtomicWrite(path, content); err != nil {
		return "", fmt.Errorf("write provider model catalog: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("secure provider model catalog: %w", err)
	}
	return path, nil
}

func readProviderCatalog(home, name string) ([]map[string]any, error) {
	if err := validateCatalogHome(home); err != nil {
		return nil, err
	}
	path := filepath.Join(home, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("inspect provider model catalog")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > proxymodel.ProviderCatalogMaxBytes {
		return nil, errors.New("provider model catalog must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open provider model catalog")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, proxymodel.ProviderCatalogMaxBytes+1))
	if err != nil || len(content) > proxymodel.ProviderCatalogMaxBytes {
		return nil, errors.New("read provider model catalog")
	}
	var envelope providerCatalogEnvelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, errors.New("parse provider model catalog")
	}
	if len(envelope.Models) > proxymodel.ProviderCatalogMaxItems {
		return nil, fmt.Errorf("provider model catalog exceeds hard limit (%d entries)", proxymodel.ProviderCatalogMaxItems)
	}
	return envelope.Models, nil
}

func validateCatalogHome(home string) error {
	if !filepath.IsAbs(home) || home == filepath.Dir(home) {
		return errors.New("provider model catalog home must be an absolute profile directory")
	}
	info, err := os.Lstat(home)
	if err != nil {
		return errors.New("inspect provider model catalog home")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("provider model catalog home must be a real directory")
	}
	return nil
}

func prepareCatalogTarget(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("inspect provider model catalog target")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if !info.Mode().IsRegular() {
		return errors.New("provider model catalog target must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return nil
}
