package runtime

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const (
	openAICompatibleProviderID   = "godex-openai-compatible"
	openAICompatibleProviderName = "OpenAI-compatible"
)

func (runner *Runner) RunOpenAICompatibleProfile(
	ctx context.Context,
	codexHome, baseURL string,
	arguments []string,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	prepared, err := openAICompatibleArguments(baseURL, arguments)
	if err != nil {
		return err
	}
	return runner.process.Run(ctx, home, prepared)
}

func openAICompatibleArguments(baseURL string, arguments []string) ([]string, error) {
	if _, found := providerConfigValue(arguments, "model_provider"); found {
		return append([]string(nil), arguments...), nil
	}
	validated, err := openAICompatibleBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	prefix := "model_providers." + openAICompatibleProviderID
	entries := []string{
		"model_provider=" + strconv.Quote(openAICompatibleProviderID),
		prefix + ".name=" + strconv.Quote(openAICompatibleProviderName),
		prefix + ".base_url=" + strconv.Quote(validated),
		prefix + `.wire_api="responses"`,
		prefix + ".requires_openai_auth=true",
		prefix + ".supports_websockets=false",
	}
	managed := make([]string, 0, len(entries)*2+len(arguments))
	for _, entry := range entries {
		managed = append(managed, "-c", entry)
	}
	return append(managed, arguments...), nil
}

func openAICompatibleBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("profile OpenAI-compatible base URL must be credential-free http(s) with no query or fragment")
	}
	return value, nil
}
