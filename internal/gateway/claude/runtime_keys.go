package claude

import (
	"errors"
	"os"
	"strings"
	"unicode"
)

const (
	anthropicAPIKeysEnv = "ANTHROPIC_API_KEYS"
	anthropicAPIKeyEnv  = "ANTHROPIC_API_KEY"
)

func ResolveRuntimeAPIKeys(explicit string) ([]string, error) {
	return resolveRuntimeAPIKeys(explicit, os.LookupEnv)
}

func (source *Source) AnthropicAPIKeys(explicit string) ([]string, error) {
	return ResolveRuntimeAPIKeys(explicit)
}

func resolveRuntimeAPIKeys(explicit string, lookup func(string) (string, bool)) ([]string, error) {
	if explicit != "" {
		key, err := runtimeSingleAPIKey(explicit, "--api-key")
		if err != nil {
			return nil, err
		}
		return []string{key}, nil
	}
	if value, ok := lookup(anthropicAPIKeysEnv); ok {
		keys := runtimeAPIKeyList(value)
		if len(keys) == 0 {
			return nil, errors.New(anthropicAPIKeysEnv + " cannot be empty")
		}
		return keys, nil
	}
	if value, ok := lookup(anthropicAPIKeyEnv); ok {
		key, err := runtimeSingleAPIKey(value, anthropicAPIKeyEnv)
		if err != nil {
			return nil, err
		}
		return []string{key}, nil
	}
	return nil, nil
}

func runtimeAPIKeyList(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';' || current == '\n'
	})
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if key := strings.TrimSpace(part); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func runtimeSingleAPIKey(value, name string) (string, error) {
	if value == "" {
		return "", errors.New(name + " cannot be empty")
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", errors.New(name + " must not contain whitespace")
	}
	return value, nil
}
