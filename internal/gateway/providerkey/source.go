package providerkey

import (
	"errors"
	"os"
	"strings"
	"unicode"
)

type Source struct {
	lookup func(string) (string, bool)
}

type envPlan struct {
	plural []string
	single []string
}

func NewSource() *Source { return &Source{lookup: os.LookupEnv} }

func (source *Source) APIKeys(provider, explicit string) ([]string, error) {
	plan, err := providerEnvPlan(provider)
	if err != nil {
		return nil, err
	}
	return resolve(explicit, plan, source.lookup)
}

func providerEnvPlan(provider string) (envPlan, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return envPlan{plural: []string{"ANTHROPIC_API_KEYS"}, single: []string{"ANTHROPIC_API_KEY"}}, nil
	case "deepseek":
		return envPlan{plural: []string{"DEEPSEEK_API_KEYS"}, single: []string{"DEEPSEEK_API_KEY"}}, nil
	case "gemini":
		return envPlan{
			plural: []string{"GEMINI_API_KEYS", "GOOGLE_API_KEYS"},
			single: []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		}, nil
	default:
		return envPlan{}, errors.New("runtime provider API-key shortcut is not implemented")
	}
}

func resolve(explicit string, plan envPlan, lookup func(string) (string, bool)) ([]string, error) {
	if explicit != "" {
		key, err := single(explicit, "--api-key")
		if err != nil {
			return nil, err
		}
		return []string{key}, nil
	}
	for _, name := range plan.plural {
		if value, ok := lookup(name); ok {
			keys := list(value)
			if len(keys) == 0 {
				return nil, errors.New(name + " cannot be empty")
			}
			return keys, nil
		}
	}
	for _, name := range plan.single {
		if value, ok := lookup(name); ok {
			key, err := single(value, name)
			if err != nil {
				return nil, err
			}
			return []string{key}, nil
		}
	}
	return nil, nil
}

func list(value string) []string {
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

func single(value, name string) (string, error) {
	if value == "" {
		return "", errors.New(name + " cannot be empty")
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", errors.New(name + " must not contain whitespace")
	}
	return value, nil
}
