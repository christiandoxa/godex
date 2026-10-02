package profile

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/pelletier/go-toml/v2"
)

const (
	profileLocalConfigFileName = ".prodex-profile.toml"
	maxProfileLocalConfigBytes = 64 << 10
)

type profileLocalConfig struct {
	OpenAICompatibleBaseURL *string `toml:"openai_compatible_base_url,omitempty"`
}

func (store *Store) ReadOpenAICompatibleBaseURL(codexHome string) (string, bool, error) {
	path := filepath.Join(codexHome, profileLocalConfigFileName)
	content, found, err := readProfileLocalConfig(path)
	if err != nil || !found {
		return "", found, err
	}
	var config profileLocalConfig
	if err := toml.Unmarshal(content, &config); err != nil {
		return "", false, errors.New("failed to parse profile local config")
	}
	if config.OpenAICompatibleBaseURL == nil {
		return "", false, nil
	}
	value, err := validateOpenAICompatibleBaseURL(*config.OpenAICompatibleBaseURL)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func writeOpenAICompatibleBaseURL(home string, baseURL *string) error {
	path := filepath.Join(home, profileLocalConfigFileName)
	if baseURL == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("failed to remove profile local config")
		}
		return nil
	}
	value, err := validateOpenAICompatibleBaseURL(*baseURL)
	if err != nil {
		return err
	}
	content, err := toml.Marshal(profileLocalConfig{OpenAICompatibleBaseURL: &value})
	if err != nil {
		return errors.New("failed to serialize profile local config")
	}
	if len(content) == 0 || len(content) > maxProfileLocalConfigBytes {
		return errors.New("profile local config exceeds the safe size limit")
	}
	if err := ensureDirectory(home); err != nil {
		return err
	}
	_, err = fileutil.AtomicWrite(path, content)
	return err
}

func readProfileLocalConfig(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("failed to inspect profile local config")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProfileLocalConfigBytes {
		return nil, false, errors.New("profile local config must be a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, false, errors.New("profile local config must be private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxProfileLocalConfigBytes {
		return nil, false, errors.New("failed to read profile local config")
	}
	return content, true, nil
}

func validateOpenAICompatibleBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("profile OpenAI-compatible base URL cannot be empty")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("profile OpenAI-compatible base URL must be credential-free http(s) with no query or fragment")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("profile OpenAI-compatible base URL must include a host")
	}
	return value, nil
}

func profileLocalConfigBackup(home string) ([]byte, bool, error) {
	content, found, err := readProfileLocalConfig(filepath.Join(home, profileLocalConfigFileName))
	if err != nil {
		return nil, false, err
	}
	return append([]byte(nil), content...), found, nil
}

func restoreProfileLocalConfig(home string, content []byte, found bool) error {
	path := filepath.Join(home, profileLocalConfigFileName)
	if !found {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if len(content) == 0 || len(content) > maxProfileLocalConfigBytes {
		return fmt.Errorf("profile local config backup is invalid")
	}
	_, err := fileutil.AtomicWrite(path, content)
	return err
}
