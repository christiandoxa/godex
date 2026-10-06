package presidio

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/pelletier/go-toml/v2"
)

const (
	ConfigFileName          = "presidio.toml"
	DefaultAnalyzerURL      = "http://localhost:5002"
	DefaultAnonymizerURL    = "http://localhost:5001"
	DefaultLanguage         = "en"
	DefaultTimeout          = 10 * time.Second
	DefaultMaxResponseBytes = 4 * 1024 * 1024
	DefaultMaxConcurrency   = 8
	MaxTimeout              = 120 * time.Second
	MaxResponseBytes        = 16 * 1024 * 1024
	MaxConcurrency          = 64
	MaxLanguages            = 16
	MaxLanguageBytes        = 32
	MaxTrustedHosts         = 64
)

type fileConfig struct {
	Enabled                                                      bool
	AnalyzerURL, AnonymizerURL, Language, LanguageMode, FailMode string
	Languages, TrustedHosts                                      []string
	TimeoutMS                                                    uint64
	MaxResponseBytes, MaxConcurrency                             int
}

func defaultFileConfig() fileConfig {
	return fileConfig{AnalyzerURL: DefaultAnalyzerURL, AnonymizerURL: DefaultAnonymizerURL, LanguageMode: "fixed", FailMode: "open", TimeoutMS: uint64(DefaultTimeout / time.Millisecond), MaxResponseBytes: DefaultMaxResponseBytes, MaxConcurrency: DefaultMaxConcurrency}
}

func LoadConfig(root string) (proxymodel.PresidioConfig, bool, error) {
	cfg := defaultFileConfig()
	path := filepath.Join(root, ConfigFileName)
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		var values map[string]any
		if err := toml.Unmarshal(content, &values); err != nil {
			return proxymodel.PresidioConfig{}, false, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := applyFileValues(&cfg, values); err != nil {
			return proxymodel.PresidioConfig{}, false, err
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return proxymodel.PresidioConfig{}, false, err
	}
	resolved, err := validateFileConfig(cfg)
	if err != nil {
		return proxymodel.PresidioConfig{}, false, err
	}
	return resolved, cfg.Enabled, nil
}

func applyFileValues(cfg *fileConfig, values map[string]any) error {
	for key, raw := range values {
		switch key {
		case "enabled":
			value, ok := raw.(bool)
			if !ok {
				return errors.New("invalid enabled: expected boolean")
			}
			cfg.Enabled = value
		case "analyzer_url":
			value, ok := raw.(string)
			if !ok {
				return errors.New("invalid analyzer_url: expected string")
			}
			cfg.AnalyzerURL = value
		case "anonymizer_url":
			value, ok := raw.(string)
			if !ok {
				return errors.New("invalid anonymizer_url: expected string")
			}
			cfg.AnonymizerURL = value
		case "language":
			value, ok := raw.(string)
			if !ok {
				return errors.New("invalid language: expected string")
			}
			cfg.Language = value
		case "language_mode":
			value, ok := raw.(string)
			if !ok {
				return errors.New("invalid language_mode: expected string")
			}
			cfg.LanguageMode = value
		case "fail_mode":
			value, ok := raw.(string)
			if !ok {
				return errors.New("invalid fail_mode: expected string")
			}
			cfg.FailMode = value
		case "timeout_ms":
			value, ok := integerValue(raw)
			if !ok || value < 0 {
				return errors.New("invalid timeout_ms: expected integer")
			}
			cfg.TimeoutMS = uint64(value)
		case "max_response_bytes":
			value, ok := integerValue(raw)
			if !ok || value < 0 {
				return errors.New("invalid max_response_bytes: expected integer")
			}
			cfg.MaxResponseBytes = int(value)
		case "max_concurrency":
			value, ok := integerValue(raw)
			if !ok || value < 0 {
				return errors.New("invalid max_concurrency: expected integer")
			}
			cfg.MaxConcurrency = int(value)
		case "languages":
			value, ok := stringList(raw)
			if !ok {
				return errors.New("invalid languages: expected string array")
			}
			cfg.Languages = value
		case "trusted_hosts":
			value, ok := stringList(raw)
			if !ok {
				return errors.New("invalid trusted_hosts: expected string array")
			}
			cfg.TrustedHosts = value
		default:
			return fmt.Errorf("unknown Presidio config field %q", key)
		}
	}
	return nil
}

func integerValue(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		if v <= uint64(^uint64(0)>>1) {
			return int64(v), true
		}
	}
	return 0, false
}
func stringList(value any) ([]string, bool) {
	if typed, ok := value.([]string); ok {
		return typed, true
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}

func validateFileConfig(config fileConfig) (proxymodel.PresidioConfig, error) {
	if strings.TrimSpace(config.AnalyzerURL) == "" {
		config.AnalyzerURL = DefaultAnalyzerURL
	}
	if strings.TrimSpace(config.AnonymizerURL) == "" {
		config.AnonymizerURL = DefaultAnonymizerURL
	}
	if strings.TrimSpace(config.LanguageMode) == "" {
		config.LanguageMode = "fixed"
	}
	if strings.TrimSpace(config.FailMode) == "" {
		config.FailMode = "open"
	}
	if config.TimeoutMS == 0 {
		config.TimeoutMS = uint64(DefaultTimeout / time.Millisecond)
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = DefaultMaxConcurrency
	}
	if err := validateURL(config.AnalyzerURL, "analyzer_url"); err != nil {
		return proxymodel.PresidioConfig{}, err
	}
	if err := validateURL(config.AnonymizerURL, "anonymizer_url"); err != nil {
		return proxymodel.PresidioConfig{}, err
	}
	if config.TimeoutMS < 100 || config.TimeoutMS > uint64(MaxTimeout/time.Millisecond) {
		return proxymodel.PresidioConfig{}, fmt.Errorf("invalid timeout_ms: expected 100..=%d", MaxTimeout/time.Millisecond)
	}
	if config.MaxResponseBytes < 1024 || config.MaxResponseBytes > MaxResponseBytes {
		return proxymodel.PresidioConfig{}, fmt.Errorf("invalid max_response_bytes: expected 1024..=%d", MaxResponseBytes)
	}
	if config.MaxConcurrency < 1 || config.MaxConcurrency > MaxConcurrency {
		return proxymodel.PresidioConfig{}, fmt.Errorf("invalid max_concurrency: expected 1..=%d", MaxConcurrency)
	}
	failClosed := false
	switch strings.ToLower(strings.TrimSpace(config.FailMode)) {
	case "open":
	case "closed":
		failClosed = true
	default:
		return proxymodel.PresidioConfig{}, errors.New("invalid fail_mode: expected 'open' or 'closed'")
	}
	mode := strings.ToLower(strings.TrimSpace(config.LanguageMode))
	if mode != "fixed" && mode != "auto" && mode != "multi" {
		return proxymodel.PresidioConfig{}, fmt.Errorf("unknown Presidio language mode: %s", config.LanguageMode)
	}
	languages := append([]string(nil), config.Languages...)
	if len(languages) == 0 {
		if strings.TrimSpace(config.Language) != "" {
			languages = []string{config.Language}
		} else {
			languages = []string{DefaultLanguage}
		}
	}
	if len(languages) == 0 || len(languages) > MaxLanguages {
		return proxymodel.PresidioConfig{}, fmt.Errorf("invalid languages: expected 1..=%d unique bounded language tags", MaxLanguages)
	}
	seen := map[string]bool{}
	for _, language := range languages {
		if language == "" || len(language) > MaxLanguageBytes {
			return proxymodel.PresidioConfig{}, fmt.Errorf("invalid languages: expected 1..=%d unique bounded language tags", MaxLanguages)
		}
		for _, current := range language {
			if !isLanguageRune(current) {
				return proxymodel.PresidioConfig{}, fmt.Errorf("invalid languages: expected 1..=%d unique bounded language tags", MaxLanguages)
			}
		}
		if seen[language] {
			return proxymodel.PresidioConfig{}, fmt.Errorf("invalid languages: expected 1..=%d unique bounded language tags", MaxLanguages)
		}
		seen[language] = true
	}
	if mode == "fixed" && len(languages) != 1 {
		return proxymodel.PresidioConfig{}, errors.New("Fixed Presidio language mode requires exactly one language")
	}
	if len(config.TrustedHosts) > MaxTrustedHosts {
		return proxymodel.PresidioConfig{}, errors.New("invalid trusted_hosts: expected bounded exact host names or IP addresses")
	}
	trusted := make([]string, 0, len(config.TrustedHosts))
	for _, host := range config.TrustedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if !validTrustedHost(host) {
			return proxymodel.PresidioConfig{}, errors.New("invalid trusted_hosts: expected bounded exact host names or IP addresses")
		}
		trusted = append(trusted, host)
	}
	return proxymodel.PresidioConfig{AnalyzerURL: config.AnalyzerURL, AnonymizerURL: config.AnonymizerURL, Languages: languages, LanguageMode: mode, FailClosed: failClosed, TrustedHosts: trusted, Timeout: time.Duration(config.TimeoutMS) * time.Millisecond, MaxResponseBytes: config.MaxResponseBytes, MaxConcurrency: config.MaxConcurrency}, nil
}
func isLanguageRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_'
}
func validateURL(value, field string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid %s: expected an http(s) URL with host and no credentials, query, or fragment", field)
	}
	return nil
}
func validTrustedHost(host string) bool {
	if host == "" || len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, r := range host {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == ':' || r == '[' || r == ']') {
			return false
		}
	}
	return true
}
func ValidateEnterpriseEndpoints(config proxymodel.PresidioConfig) error {
	for _, current := range []struct{ value, field string }{{config.AnalyzerURL, "analyzer_url"}, {config.AnonymizerURL, "anonymizer_url"}} {
		parsed, err := url.Parse(current.value)
		if err != nil {
			return fmt.Errorf("invalid %s", current.field)
		}
		host := parsed.Hostname()
		if host == "" {
			return fmt.Errorf("untrusted %s: endpoint host is required", current.field)
		}
		if privateHost(host) {
			continue
		}
		trusted := false
		for _, candidate := range config.TrustedHosts {
			if strings.EqualFold(candidate, host) {
				trusted = true
				break
			}
		}
		if !trusted {
			return fmt.Errorf("untrusted %s: enterprise governance requires a private/on-prem endpoint or an exact trusted_hosts entry", current.field)
		}
	}
	return nil
}
func privateHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && (address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified())
}
