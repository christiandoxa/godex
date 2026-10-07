package profile

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const apiKeyDefaultProfileName = "api_key"

func (catalog *Catalog) LoginAPIKey(ctx context.Context, input profilemodel.APIKeyLoginInput) (profilemodel.APIKeyLoginResult, error) {
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		return profilemodel.APIKeyLoginResult{}, errors.New("API key cannot be empty")
	}
	baseURL, baseURLPointer, err := normalizeAPIKeyBaseURL(input.BaseURL, input.BaseURLSpecified)
	if err != nil {
		return profilemodel.APIKeyLoginResult{}, err
	}
	listed, err := catalog.List(ctx)
	if err != nil {
		return profilemodel.APIKeyLoginResult{}, err
	}
	name := apiKeyProfileName(input.Name, baseURL, listed)
	profile := profileentity.Profile{
		Name: name, CodexHome: catalog.profiles.ManagedHome(name), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := profileentity.Validate(profile); err != nil {
		return profilemodel.APIKeyLoginResult{}, err
	}
	authJSON, err := apiKeyAuthJSON(apiKey)
	if err != nil {
		return profilemodel.APIKeyLoginResult{}, err
	}
	defer clearBundleBytes(authJSON)
	stored, created, err := catalog.profiles.LoginOpenAIAPIKey(
		ctx, profile, authJSON, baseURLPointer, input.BaseURLSpecified, true,
	)
	if err != nil {
		return profilemodel.APIKeyLoginResult{}, err
	}
	return profilemodel.APIKeyLoginResult{
		Profile: stored.Name, Created: created, Active: true, BaseURL: baseURL,
	}, nil
}

func apiKeyAuthJSON(apiKey string) ([]byte, error) {
	content, err := json.MarshalIndent(map[string]string{
		"auth_mode": "apikey", "OPENAI_API_KEY": apiKey,
	}, "", "  ")
	if err != nil {
		return nil, errors.New("failed to serialize API key auth JSON")
	}
	return content, nil
}

func normalizeAPIKeyBaseURL(value string, specified bool) (string, *string, error) {
	if !specified {
		return "", nil, nil
	}
	if value == "" {
		return "", nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", nil, errors.New("profile OpenAI-compatible base URL must be credential-free http(s) with no query or fragment")
	}
	copy := value
	return value, &copy, nil
}

func apiKeyProfileName(requested, baseURL string, listed []Report) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return apiKeySanitizedName(requested)
	}
	base := apiKeyDefaultProfileName
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Hostname() != "" {
		base = sanitizeProfileSlug("api_key_" + parsed.Hostname())
		if base == "profile" {
			base = apiKeyDefaultProfileName
		}
	}
	for _, report := range listed {
		if report.Profile.Name == base {
			return base
		}
	}
	return uniqueProfileName(base, apiKeyDefaultProfileName, listed)
}

func apiKeySanitizedName(value string) string {
	name := sanitizeProfileSlug(value)
	if name == "profile" {
		return apiKeyDefaultProfileName
	}
	return name
}

func DefaultAPIKeyProfileName(baseURL string) string {
	return apiKeyProfileName("", strings.TrimSpace(baseURL), nil)
}
