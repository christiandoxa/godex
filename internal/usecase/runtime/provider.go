package runtime

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	copilotDefaultModel       = "gpt-6-astra"
	copilotContextWindow      = int64(1_050_000)
	copilotAutoCompactLimit   = int64(997_500)
	anthropicDefaultModel     = "claude-sonnet-5-5"
	anthropicContextWindow    = int64(1_000_000)
	anthropicAutoCompactLimit = int64(950_000)
	anthropicDefaultAPIURL    = "https://api.anthropic.com/v1"
	deepSeekDefaultModel      = "deepseek-v4-pro"
	deepSeekContextWindow     = int64(1_048_576)
	deepSeekAutoCompactLimit  = int64(900_000)
	deepSeekDefaultAPIURL     = "https://api.deepseek.com"
	geminiDefaultModel        = "auto"
	geminiContextWindow       = int64(1_048_576)
	geminiAutoCompactLimit    = int64(900_000)
	geminiDefaultAPIURL       = "https://generativelanguage.googleapis.com/v1beta"
	kiroDefaultModel          = "auto"
	kiroContextWindow         = int64(1_000_000)
	kiroAutoCompactLimit      = int64(950_000)
	kiroDefaultAPIURL         = "https://kiro.dev"
)

func CopilotProvider(name, host, login, apiURL string) proxymodel.Provider {
	return proxymodel.Provider{
		Kind: "copilot", Name: name, Host: strings.TrimSpace(host), Login: strings.TrimSpace(login), APIURL: strings.TrimSpace(apiURL),
		DefaultModel: copilotDefaultModel, ContextWindow: copilotContextWindow, AutoCompactLimit: copilotAutoCompactLimit,
	}
}

func AnthropicProvider(name, apiURL string) proxymodel.Provider {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		apiURL = anthropicDefaultAPIURL
	}
	return proxymodel.Provider{
		Kind: "anthropic", Name: name, APIURL: apiURL,
		DefaultModel:     anthropicDefaultModel,
		ContextWindow:    anthropicContextWindow,
		AutoCompactLimit: anthropicAutoCompactLimit,
	}
}

func DeepSeekProvider(name, apiURL string) proxymodel.Provider {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		apiURL = deepSeekDefaultAPIURL
	}
	return proxymodel.Provider{
		Kind: "deepseek", Name: name, APIURL: apiURL,
		DefaultModel:     deepSeekDefaultModel,
		ContextWindow:    deepSeekContextWindow,
		AutoCompactLimit: deepSeekAutoCompactLimit,
	}
}

func GeminiProvider(name, apiURL string) proxymodel.Provider {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		apiURL = geminiDefaultAPIURL
	}
	return proxymodel.Provider{
		Kind: "gemini", Name: name, APIURL: apiURL,
		DefaultModel:     geminiDefaultModel,
		ContextWindow:    geminiContextWindow,
		AutoCompactLimit: geminiAutoCompactLimit,
	}
}

func KiroProvider(name string) proxymodel.Provider {
	return proxymodel.Provider{
		Kind: "kiro", Name: name, APIURL: kiroDefaultAPIURL,
		DefaultModel:     kiroDefaultModel,
		ContextWindow:    kiroContextWindow,
		AutoCompactLimit: kiroAutoCompactLimit,
	}
}

func providerRuntimeArguments(provider proxymodel.Provider, arguments []string) []string {
	prepared, _ := prepareProviderRuntimeArguments(nil, "", provider, arguments)
	return prepared
}

func effectiveProviderModel(provider proxymodel.Provider, arguments []string) string {
	model := strings.TrimSpace(provider.DefaultModel)
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			break
		}
		next, handled := consumeProviderModelArgument(arguments, index, &model)
		if handled {
			index = next
			continue
		}
		index++
	}
	if model == "" {
		return strings.TrimSpace(provider.DefaultModel)
	}
	return model
}

func consumeProviderModelArgument(arguments []string, index int, model *string) (int, bool) {
	argument := arguments[index]
	if value, consumed, ok := providerConfigArgument(arguments, index); ok {
		if key, scalar, valid := providerConfigAssignment(value); valid && key == "model" {
			*model = strings.TrimSpace(scalar)
		}
		return index + consumed, true
	}
	if argument == "--model" || argument == "-m" {
		if index+1 < len(arguments) {
			*model = strings.TrimSpace(arguments[index+1])
			return index + 2, true
		}
		return index + 1, true
	}
	if strings.HasPrefix(argument, "--model=") || strings.HasPrefix(argument, "-m=") {
		_, value, _ := strings.Cut(argument, "=")
		*model = strings.TrimSpace(value)
		return index + 1, true
	}
	return index, false
}

func effectiveProviderUint(arguments []string, key string, fallback uint64) (uint64, error) {
	value, found := providerConfigOverride(arguments, key)
	if !found {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("provider config %s must be a positive integer", key)
	}
	return parsed, nil
}

func providerConfigValue(arguments []string, key string) (string, bool) {
	return providerConfigOverride(arguments, key)
}

func providerConfigOverride(arguments []string, key string) (string, bool) {
	var value string
	found := false
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			break
		}
		assignment, consumed, ok := providerConfigArgument(arguments, index)
		if !ok {
			index++
			continue
		}
		if candidate, scalar, valid := providerConfigAssignment(assignment); valid && candidate == key {
			value, found = scalar, true
		}
		index += consumed
	}
	return value, found
}

func providerConfigArgument(arguments []string, index int) (string, int, bool) {
	argument := arguments[index]
	if argument == "-c" || argument == "--config" {
		if index+1 >= len(arguments) {
			return "", 1, true
		}
		return arguments[index+1], 2, true
	}
	if strings.HasPrefix(argument, "--config=") {
		return strings.TrimPrefix(argument, "--config="), 1, true
	}
	if strings.HasPrefix(argument, "-c") && argument != "-C" {
		value := strings.TrimPrefix(argument, "-c")
		value = strings.TrimPrefix(value, "=")
		return value, 1, true
	}
	return "", 1, false
}

func providerConfigAssignment(value string) (string, string, bool) {
	key, scalar, found := strings.Cut(strings.TrimSpace(value), "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if len(key) >= 2 && key[0] == '"' && key[len(key)-1] == '"' {
		if decoded, err := strconv.Unquote(key); err == nil {
			key = decoded
		}
	}
	key = strings.Trim(key, "'")
	parsed, err := providerScalar(strings.TrimSpace(scalar))
	if err != nil {
		return key, strings.TrimSpace(scalar), true
	}
	return key, parsed, true
}

func providerScalar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '"' {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return decoded, nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated literal string")
		}
		return value[1 : len(value)-1], nil
	}
	return value, nil
}
