package codex

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type UsageAuth struct {
	AccessToken string
	AccountID   string
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
	var auth codexAuthFile
	if err := json.Unmarshal(content, &auth); err != nil {
		return UsageAuth{}, errors.New("decode Codex auth profile")
	}
	defer clearAuthTokens(&auth)
	mode := strings.ToLower(strings.TrimSpace(auth.AuthMode))
	if mode != "chatgpt" {
		return UsageAuth{}, errors.New("codex profile is not configured for Sign in with ChatGPT")
	}
	token := strings.TrimSpace(auth.Tokens.AccessToken)
	if token == "" {
		return UsageAuth{}, errors.New("codex auth profile has no access token")
	}
	accountID := strings.TrimSpace(auth.Tokens.AccountID)
	if claims, err := decodeJWTPayload(token); err == nil {
		if fromClaims := firstString(
			claims["chatgpt_account_id"],
			claims["https://api.openai.com/auth.chatgpt_account_id"],
			nestedValue(claims, "https://api.openai.com/auth", "chatgpt_account_id"),
		); fromClaims != "" {
			accountID = fromClaims
		}
	}
	return UsageAuth{AccessToken: token, AccountID: accountID}, nil
}
