package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

type codexAuthFile struct {
	AuthMode     string `json:"auth_mode"`
	OpenAIAPIKey string `json:"OPENAI_API_KEY"`
	Tokens       struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

const maxAuthFileSize = 1 << 20

// ReadAccessToken is the narrow runtime handoff to the proxy. The token never
// appears in an error or diagnostic; Codex remains the owner of the file.
func ReadAccessToken(path string) (string, error) {
	content, err := readPrivateAuthFile(path)
	if err != nil {
		return "", err
	}
	defer clear(content)

	var auth codexAuthFile
	if err := json.Unmarshal(content, &auth); err != nil {
		return "", errors.New("decode Codex auth profile")
	}
	defer clearAuthTokens(&auth)
	if mode := strings.ToLower(strings.TrimSpace(auth.AuthMode)); mode != "chatgpt" {
		return "", errors.New("codex profile is not configured for Sign in with ChatGPT")
	}
	token := strings.TrimSpace(auth.Tokens.AccessToken)
	if token == "" {
		return "", errors.New("codex auth profile has no access token")
	}
	return token, nil
}

func readChatGPTIdentity(path string) (entity.Identity, error) {
	content, err := readPrivateAuthFile(path)
	if err != nil {
		return entity.Identity{}, err
	}
	defer clear(content)

	return chatGPTIdentity(content)
}

func chatGPTIdentity(content []byte) (entity.Identity, error) {
	var auth codexAuthFile
	if err := json.Unmarshal(content, &auth); err != nil {
		return entity.Identity{}, errors.New("decode Codex auth profile")
	}
	defer clearAuthTokens(&auth)
	if mode := strings.ToLower(strings.TrimSpace(auth.AuthMode)); mode != "chatgpt" {
		return entity.Identity{}, errors.New("codex profile is not configured for Sign in with ChatGPT")
	}
	if auth.Tokens.AccessToken == "" && auth.Tokens.IDToken == "" {
		return entity.Identity{}, errors.New("codex auth profile does not contain ChatGPT tokens")
	}

	identity := entity.Identity{}
	for _, token := range []string{auth.Tokens.IDToken, auth.Tokens.AccessToken} {
		if strings.TrimSpace(token) == "" {
			continue
		}
		claims, err := decodeJWTPayload(token)
		if err != nil {
			continue
		}
		mergeIdentity(&identity, claims)
	}
	if identity.ChatGPTAccountID == "" {
		identity.ChatGPTAccountID = strings.TrimSpace(auth.Tokens.AccountID)
	}
	identity.Email = strings.ToLower(strings.TrimSpace(identity.Email))
	if identity.Email == "" && identity.ChatGPTAccountID == "" {
		return entity.Identity{}, errors.New("codex auth profile does not expose a usable ChatGPT identity")
	}
	return identity, nil
}

func clearAuthTokens(auth *codexAuthFile) {
	auth.Tokens.IDToken = ""
	auth.Tokens.AccessToken = ""
	auth.Tokens.RefreshToken = ""
	auth.Tokens.AccountID = ""
}

func readPrivateAuthFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read Codex auth profile: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("codex auth profile is not a regular file")
	}
	if info.Size() > maxAuthFileSize {
		return nil, errors.New("codex auth profile is too large")
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure Codex auth profile: %w", err)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read Codex auth profile: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxAuthFileSize+1))
	closeErr := file.Close()
	if err != nil {
		clear(content)
		return nil, fmt.Errorf("read Codex auth profile: %w", err)
	}
	if closeErr != nil {
		clear(content)
		return nil, fmt.Errorf("close Codex auth profile: %w", closeErr)
	}
	if len(content) > maxAuthFileSize {
		clear(content)
		return nil, errors.New("codex auth profile is too large")
	}
	return content, nil
}

// decodeJWTPayload decodes unverified claims for display and deduplication only.
func decodeJWTPayload(token string) (map[string]any, error) {
	token = strings.TrimSpace(token)
	if len(token) > maxAuthFileSize {
		return nil, errors.New("JWT is too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, errors.New("JWT payload is not valid base64url")
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil || claims == nil {
		return nil, errors.New("JWT payload is not valid JSON")
	}
	return claims, nil
}

func mergeIdentity(identity *entity.Identity, claims map[string]any) {
	if identity.Email == "" {
		identity.Email = firstString(
			claims["email"],
			claims["https://api.openai.com/profile.email"],
			nestedValue(claims, "https://api.openai.com/profile", "email"),
		)
	}
	if identity.ChatGPTAccountID == "" {
		identity.ChatGPTAccountID = firstString(
			claims["chatgpt_account_id"],
			claims["https://api.openai.com/auth.chatgpt_account_id"],
			nestedValue(claims, "https://api.openai.com/auth", "chatgpt_account_id"),
		)
	}
}

func nestedValue(values map[string]any, namespace, key string) any {
	nested, ok := values[namespace].(map[string]any)
	if !ok {
		return nil
	}
	return nested[key]
}

func firstString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}
