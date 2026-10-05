package codex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type UsageAuth struct {
	AccessToken  string
	AccountID    string
	RefreshToken string
	ExpiresAt    *int64
	LastRefresh  *int64
}

type usageStoredAuth struct {
	AuthMode      *string `json:"auth_mode"`
	OpenAIAPIKey  *string `json:"OPENAI_API_KEY"`
	BedrockAPIKey *struct {
		APIKey *string `json:"api_key"`
	} `json:"bedrock_api_key"`
	Tokens      *usageStoredTokens `json:"tokens"`
	LastRefresh *string            `json:"last_refresh"`
}

type usageStoredTokens struct {
	AccessToken  *string `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
	AccountID    *string `json:"account_id"`
}

func ReadUsageAuth(codexHome string) (UsageAuth, error) {
	content, err := readPrivateAuthFile(filepath.Join(codexHome, "auth.json"))
	if err != nil {
		return UsageAuth{}, err
	}
	defer clear(content)
	return decodeUsageAuth(content)
}

func decodeUsageAuth(content []byte) (UsageAuth, error) {
	var stored usageStoredAuth
	if err := json.Unmarshal(content, &stored); err != nil {
		return UsageAuth{}, errors.New("decode Codex auth profile")
	}
	defer clearUsageStoredAuth(&stored)
	if usageAuthProviderManaged(stored) {
		if usageAuthModeMatches(stored.AuthMode, "bedrockapikey") ||
			(stored.BedrockAPIKey != nil && nonemptyUsageSecret(stored.BedrockAPIKey.APIKey)) {
			return UsageAuth{}, errors.New("quota endpoint requires a ChatGPT access token. Amazon Bedrock API key auth is provider-managed.")
		}
		return UsageAuth{}, errors.New("quota endpoint requires a ChatGPT access token. Run `codex login` first.")
	}
	if stored.Tokens == nil {
		return UsageAuth{}, errors.New("auth tokens are missing from the stored auth secret")
	}
	token := normalizedUsageSecret(stored.Tokens.AccessToken)
	if token == "" {
		return UsageAuth{}, errors.New("access token not found in the stored auth secret")
	}
	accountID := normalizedUsageSecret(stored.Tokens.AccountID)
	var expiresAt *int64
	if claims, err := decodeUsageJWTClaims(token); err == nil {
		if fromClaims := usageJWTAccountID(claims); fromClaims != "" {
			accountID = fromClaims
		}
		expiresAt = usageJWTExpiration(claims)
	}
	refreshToken := normalizedUsageSecret(stored.Tokens.RefreshToken)
	lastRefresh := parseUsageLastRefresh(stored.LastRefresh)
	return UsageAuth{
		AccessToken: token, AccountID: accountID, RefreshToken: refreshToken,
		ExpiresAt: expiresAt, LastRefresh: lastRefresh,
	}, nil
}

func usageAuthProviderManaged(stored usageStoredAuth) bool {
	return nonemptyUsageSecret(stored.OpenAIAPIKey) ||
		(stored.BedrockAPIKey != nil && nonemptyUsageSecret(stored.BedrockAPIKey.APIKey)) ||
		usageAuthModeMatches(stored.AuthMode, "apikey") ||
		usageAuthModeMatches(stored.AuthMode, "bedrockapikey")
}

func usageAuthModeMatches(value *string, target string) bool {
	if value == nil {
		return false
	}
	var normalized strings.Builder
	for _, current := range strings.TrimSpace(*value) {
		switch current {
		case ' ', '-', '_':
			continue
		}
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		if current >= 0x80 {
			return false
		}
		normalized.WriteRune(current)
	}
	return normalized.String() == target
}

func nonemptyUsageSecret(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func normalizedUsageSecret(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func clearUsageStoredAuth(stored *usageStoredAuth) {
	if stored == nil {
		return
	}
	if stored.OpenAIAPIKey != nil {
		*stored.OpenAIAPIKey = "<redacted>"
	}
	if stored.BedrockAPIKey != nil && stored.BedrockAPIKey.APIKey != nil {
		*stored.BedrockAPIKey.APIKey = "<redacted>"
	}
	if stored.Tokens != nil {
		if stored.Tokens.AccessToken != nil {
			*stored.Tokens.AccessToken = "<redacted>"
		}
		if stored.Tokens.RefreshToken != nil {
			*stored.Tokens.RefreshToken = "<redacted>"
		}
		if stored.Tokens.AccountID != nil {
			*stored.Tokens.AccountID = "<redacted>"
		}
	}
}

func decodeUsageJWTClaims(token string) (map[string]any, error) {
	if len(token) > maxAuthFileSize {
		return nil, errors.New("JWT is too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || base64.RawURLEncoding.EncodeToString(payload) != parts[1] {
		return nil, errors.New("non-canonical JWT payload encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var claims map[string]any
	if err := decoder.Decode(&claims); err != nil || claims == nil {
		return nil, errors.New("failed to parse JWT payload JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("failed to parse JWT payload JSON")
	}
	return claims, nil
}

func usageJWTAccountID(claims map[string]any) string {
	return firstString(
		nestedValue(claims, "https://api.openai.com/auth", "chatgpt_account_id"),
		claims["https://api.openai.com/auth.chatgpt_account_id"],
		claims["chatgpt_account_id"],
	)
}

func usageJWTExpiration(claims map[string]any) *int64 {
	number, ok := claims["exp"].(json.Number)
	if !ok {
		return nil
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func parseUsageLastRefresh(value *string) *int64 {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil
	}
	epoch := parsed.Unix()
	return &epoch
}
