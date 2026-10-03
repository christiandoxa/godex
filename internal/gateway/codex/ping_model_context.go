package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/pelletier/go-toml/v2"
)

const pingCodexConfigMaxBytes int64 = 1024 * 1024

type pingModelConfig struct {
	values map[string]any
}

func pingModelContextArguments(codexHome string, args []string) ([]string, error) {
	model := pingCLIModel(args)
	var config pingModelConfig
	var err error
	if model == "" {
		config, err = readPingModelConfig(filepath.Join(codexHome, "config.toml"))
		if err != nil {
			return nil, err
		}
		model = config.stringValue("model")
	}
	if model == "" || !pingOpenAILargeContextModel(model) {
		return append([]string(nil), args...), nil
	}
	if config.values == nil {
		config, err = readPingModelConfig(filepath.Join(codexHome, "config.toml"))
		if err != nil {
			return nil, err
		}
	}

	explicitContext := config.uintValue("model_context_window")
	if explicitContext == nil && pingOpenAIConfigProvider(config.stringValue("model_provider")) {
		if configuredModel := config.stringValue("model"); configuredModel != "" {
			explicitContext = pingOpenAIModelContextFromCache(codexHome, configuredModel)
		}
	}

	contextWindow := explicitContext
	if contextWindow == nil {
		contextWindow = pingOpenAIModelContextFromCache(codexHome, model)
	}
	if contextWindow == nil {
		contextWindow, err = pingOpenAIModelContextFromCatalog(model)
		if err != nil {
			return nil, err
		}
	}
	if contextWindow == nil {
		return append([]string(nil), args...), nil
	}

	overrides := make([]string, 0, 4)
	if explicitContext == nil {
		overrides = append(overrides, "-c", "model_context_window="+strconv.FormatUint(*contextWindow, 10))
	}
	if config.uintValue("model_auto_compact_token_limit") == nil {
		overrides = append(overrides, "-c", "model_auto_compact_token_limit="+strconv.FormatUint(*contextWindow*9/10, 10))
	}
	if len(overrides) == 0 {
		return append([]string(nil), args...), nil
	}
	return append(overrides, args...), nil
}

func readPingModelConfig(path string) (pingModelConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pingModelConfig{}, nil
		}
		return pingModelConfig{}, fmt.Errorf("read Codex config: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, pingCodexConfigMaxBytes+1))
	if err != nil {
		return pingModelConfig{}, fmt.Errorf("read Codex config: %w", err)
	}
	if int64(len(content)) > pingCodexConfigMaxBytes {
		return pingModelConfig{}, fmt.Errorf("read Codex config: config exceeds safe size limit (%d bytes)", pingCodexConfigMaxBytes)
	}
	values := make(map[string]any)
	if err := toml.Unmarshal(content, &values); err != nil {
		return pingModelConfig{}, fmt.Errorf("parse Codex config: %w", err)
	}
	return pingModelConfig{values: values}, nil
}

func (config pingModelConfig) stringValue(key string) string {
	if config.values == nil {
		return ""
	}
	value, ok := config.values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func (config pingModelConfig) uintValue(key string) *uint64 {
	if config.values == nil {
		return nil
	}
	return pingModelUint(config.values[key])
}

func pingModelUint(value any) *uint64 {
	var parsed uint64
	switch current := value.(type) {
	case int64:
		if current <= 1 {
			return nil
		}
		parsed = uint64(current)
	case uint64:
		if current <= 1 {
			return nil
		}
		parsed = current
	case string:
		number, err := strconv.ParseUint(strings.TrimSpace(current), 10, 64)
		if err != nil || number <= 1 {
			return nil
		}
		parsed = number
	default:
		return nil
	}
	return &parsed
}

func pingCLIModel(args []string) string {
	for index := 0; index < len(args); index++ {
		if args[index] == "--" {
			break
		}
		switch {
		case args[index] == "--model" && index+1 < len(args):
			return args[index+1]
		case strings.HasPrefix(args[index], "--model="):
			return strings.TrimPrefix(args[index], "--model=")
		}
	}
	return ""
}

func pingOpenAIConfigProvider(provider string) bool {
	return provider == "" || strings.EqualFold(strings.TrimSpace(provider), "openai")
}

func pingOpenAILargeContextModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "gpt-5") {
		return true
	}
	switch model {
	case "gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "codex-auto-review":
		return true
	default:
		return false
	}
}

func pingOpenAIPreferMaxContextModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna",
		"gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna":
		return true
	default:
		return false
	}
}

func pingOpenAIModelContextFromCache(codexHome, model string) *uint64 {
	content, err := os.ReadFile(filepath.Join(codexHome, "models_cache.json"))
	if err != nil || !utf8.Valid(content) {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var document map[string]any
	if decoder.Decode(&document) != nil {
		return nil
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil
	}
	entries, ok := document["models"].([]any)
	if !ok {
		return nil
	}
	query := strings.TrimSpace(model)
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		identity, hasSlug := entry["slug"].(string)
		if !hasSlug {
			identity, _ = entry["id"].(string)
		}
		identity = strings.TrimSpace(identity)
		if identity == "" || !pingASCIIEqualFold(identity, query) {
			continue
		}
		context := pingJSONUint(entry["context_window"])
		var maxContext *uint64
		if value, exists := entry["max_context_window"]; exists {
			maxContext = pingJSONUint(value)
		} else {
			maxContext = pingJSONUint(entry["max_context_window_tokens"])
		}
		if pingOpenAIPreferMaxContextModel(model) {
			if maxContext != nil {
				return maxContext
			}
			return context
		}
		if context != nil {
			return context
		}
		return maxContext
	}
	return nil
}

func pingASCIIEqualFold(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := 0; index < len(left); index++ {
		leftByte, rightByte := left[index], right[index]
		if leftByte >= 'A' && leftByte <= 'Z' {
			leftByte += 'a' - 'A'
		}
		if rightByte >= 'A' && rightByte <= 'Z' {
			rightByte += 'a' - 'A'
		}
		if leftByte != rightByte {
			return false
		}
	}
	return true
}

func pingJSONUint(value any) *uint64 {
	number, ok := value.(json.Number)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil || parsed <= 1 {
		return nil
	}
	return &parsed
}

func pingOpenAIModelContextFromCatalog(model string) (*uint64, error) {
	entries, err := proxymodel.OpenAIProviderCatalog()
	if err != nil {
		return nil, err
	}
	entry := proxymodel.ResolveProviderCatalogEntry(entries, model)
	if entry == nil || entry.ContextWindowTokens == nil || *entry.ContextWindowTokens <= 1 {
		return nil, nil
	}
	value := *entry.ContextWindowTokens
	return &value, nil
}
