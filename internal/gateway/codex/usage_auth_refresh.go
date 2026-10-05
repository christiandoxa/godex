package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
)

type ChatGPTRefreshResponse struct {
	IDToken      *string
	AccessToken  *string
	RefreshToken *string
}

func UsageAuthNeedsProactiveRefresh(
	auth UsageAuth,
	now, expirySkewSeconds, refreshIntervalDays int64,
) bool {
	if auth.ExpiresAt != nil {
		threshold := saturatingUsageAuthAdd(now, expirySkewSeconds)
		return *auth.ExpiresAt <= threshold
	}
	if auth.LastRefresh == nil {
		return false
	}
	elapsed := new(big.Int).Sub(big.NewInt(now), big.NewInt(*auth.LastRefresh))
	interval := new(big.Int).Mul(big.NewInt(refreshIntervalDays), big.NewInt(86_400))
	return elapsed.Cmp(interval) >= 0
}

func saturatingUsageAuthAdd(left, right int64) int64 {
	switch {
	case right > 0 && left > math.MaxInt64-right:
		return math.MaxInt64
	case right < 0 && left < math.MinInt64-right:
		return math.MinInt64
	default:
		return left + right
	}
}

func ApplyChatGPTRefresh(
	content []byte,
	refreshed ChatGPTRefreshResponse,
	refreshedAt string,
) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, errors.New("failed to parse stored auth JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("failed to parse stored auth JSON")
	}
	auth, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("stored auth JSON must be an object")
	}
	tokens, err := usageAuthTokensObject(auth)
	if err != nil {
		return nil, err
	}

	if refreshed.IDToken != nil {
		tokens["id_token"] = *refreshed.IDToken
	}
	var refreshedAccount string
	if refreshed.AccessToken != nil {
		tokens["access_token"] = *refreshed.AccessToken
		if claims, err := decodeUsageJWTClaims(*refreshed.AccessToken); err == nil {
			refreshedAccount = usageJWTAccountID(claims)
		}
	}
	if refreshedAccount != "" {
		tokens["account_id"] = refreshedAccount
	}
	if refreshed.RefreshToken != nil {
		tokens["refresh_token"] = *refreshed.RefreshToken
	}
	auth["last_refresh"] = refreshedAt
	return marshalUsageAuthJSON(auth)
}

func usageAuthTokensObject(auth map[string]any) (map[string]any, error) {
	value, exists := auth["tokens"]
	if !exists {
		tokens := make(map[string]any)
		auth["tokens"] = tokens
		return tokens, nil
	}
	tokens, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("stored auth tokens must be an object")
	}
	return tokens, nil
}

func marshalUsageAuthJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, errors.New("failed to serialize stored auth JSON")
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{0x0a}), nil
}
