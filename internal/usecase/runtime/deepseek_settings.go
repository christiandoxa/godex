package runtime

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/pelletier/go-toml/v2"
)

const deepSeekConfigMaxBytes = 1 << 20

const (
	deepSeekSettingEmptyFormat      = "%s cannot be empty"
	deepSeekSettingWhitespaceFormat = "%s must not contain whitespace"
)

func applyDeepSeekRuntimeSettings(home string, provider *proxymodel.Provider) error {
	if provider == nil {
		return errors.New("DeepSeek runtime provider is unavailable")
	}
	settings, err := deepSeekRuntimeSettings(home, os.LookupEnv)
	if err != nil {
		return err
	}
	provider.StrictTools = settings.strictTools
	provider.WebSearchMode = settings.webSearchMode
	provider.BetaBaseURL = settings.betaBaseURL
	provider.SSELookaheadTimeout = settings.sseLookaheadTimeout
	return nil
}

type deepSeekSettings struct {
	strictTools         bool
	webSearchMode       string
	betaBaseURL         string
	sseLookaheadTimeout time.Duration
}

func lookupDeepSeekRuntimeEnv(lookup func(string) (string, bool), canonical, legacy string) (string, bool) {
	if value, found := lookup(canonical); found {
		return value, true
	}
	return lookup(legacy)
}

func deepSeekRuntimeSettings(home string, lookup func(string) (string, bool)) (deepSeekSettings, error) {
	config := readDeepSeekConfig(home)
	strict, err := deepSeekStrictTools(config, lookup)
	if err != nil {
		return deepSeekSettings{}, err
	}
	mode, err := deepSeekWebSearchMode(config, lookup)
	if err != nil {
		return deepSeekSettings{}, err
	}
	beta, err := deepSeekBetaBaseURL(config, lookup)
	if err != nil {
		return deepSeekSettings{}, err
	}
	lookahead, err := deepSeekSSELookaheadTimeout(lookup)
	if err != nil {
		return deepSeekSettings{}, err
	}
	return deepSeekSettings{
		strictTools: strict, webSearchMode: mode, betaBaseURL: beta,
		sseLookaheadTimeout: lookahead,
	}, nil
}

func deepSeekSSELookaheadTimeout(lookup func(string) (string, bool)) (time.Duration, error) {
	const key = "GODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS"
	value, found := lookupDeepSeekRuntimeEnv(lookup, key, "PRODEX_RUNTIME_PROXY_SSE_LOOKAHEAD_TIMEOUT_MS")
	if !found {
		return time.Second, nil
	}
	milliseconds, err := strconv.ParseUint(value, 10, 63)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", key)
	}
	if milliseconds == 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	if milliseconds > uint64((1<<63-1)/int64(time.Millisecond)) {
		return 0, fmt.Errorf("%s exceeds the supported duration", key)
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

func readDeepSeekConfig(home string) map[string]any {
	if strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return nil
	}
	path := filepath.Join(filepath.Clean(home), "config.toml")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, deepSeekConfigMaxBytes+1))
	if err != nil || len(content) > deepSeekConfigMaxBytes {
		return nil
	}
	var root map[string]any
	if toml.Unmarshal(content, &root) != nil {
		return nil
	}
	deepseek, _ := root["deepseek"].(map[string]any)
	return deepseek
}

func deepSeekStrictTools(config map[string]any, lookup func(string) (string, bool)) (bool, error) {
	if config != nil {
		if value, found := config["strict_tools"]; found {
			switch typed := value.(type) {
			case bool:
				return typed, nil
			case string:
				return deepSeekBool("deepseek.strict_tools", typed)
			default:
				return false, errors.New("deepseek.strict_tools must be a boolean")
			}
		}
	}
	const key = "GODEX_DEEPSEEK_STRICT_TOOLS"
	value, found := lookupDeepSeekRuntimeEnv(lookup, key, "PRODEX_DEEPSEEK_STRICT_TOOLS")
	if !found {
		return false, nil
	}
	return deepSeekBool(key, value)
}

func deepSeekWebSearchMode(config map[string]any, lookup func(string) (string, bool)) (string, error) {
	if config != nil {
		if value, found := config["web_search_mode"]; found {
			text, ok := value.(string)
			if !ok {
				return "", errors.New("deepseek.web_search_mode must be a string")
			}
			return deepSeekWebSearchValue("deepseek.web_search_mode", text)
		}
	}
	const key = "GODEX_DEEPSEEK_WEB_SEARCH_MODE"
	value, found := lookupDeepSeekRuntimeEnv(lookup, key, "PRODEX_DEEPSEEK_WEB_SEARCH_MODE")
	if !found {
		return "auto", nil
	}
	return deepSeekWebSearchValue(key, value)
}

func deepSeekBetaBaseURL(config map[string]any, lookup func(string) (string, bool)) (string, error) {
	if config != nil {
		if value, found := config["beta_base_url"]; found {
			text, ok := value.(string)
			if !ok {
				return "", errors.New("deepseek.beta_base_url must be a string")
			}
			return validateDeepSeekURL("deepseek.beta_base_url", text)
		}
	}
	const key = "GODEX_DEEPSEEK_BETA_BASE_URL"
	value, found := lookupDeepSeekRuntimeEnv(lookup, key, "PRODEX_DEEPSEEK_BETA_BASE_URL")
	if !found {
		return "https://api.deepseek.com/beta", nil
	}
	return validateDeepSeekURL(key, value)
}

func deepSeekBool(name, value string) (bool, error) {
	if value == "" {
		return false, fmt.Errorf(deepSeekSettingEmptyFormat, name)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false, fmt.Errorf(deepSeekSettingWhitespaceFormat, name)
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func deepSeekWebSearchValue(name, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf(deepSeekSettingEmptyFormat, name)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf(deepSeekSettingWhitespaceFormat, name)
	}
	value = strings.ToLower(value)
	switch value {
	case "auto", "off", "openai_chat", "anthropic":
		return value, nil
	default:
		return "", fmt.Errorf("%s must be auto, off, openai_chat, or anthropic", name)
	}
}

func validateDeepSeekURL(name, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf(deepSeekSettingEmptyFormat, name)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf(deepSeekSettingWhitespaceFormat, name)
	}
	normalized := strings.TrimRight(value, "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%s must be an http(s) URL with host and no credentials, query, or fragment", name)
	}
	return normalized, nil
}
