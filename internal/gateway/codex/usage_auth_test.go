package codex

import (
	"path/filepath"
	"testing"
)

func TestReadUsageAuthPrefersAccountIDFromAccessToken(t *testing.T) {
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"account_id": "stored-account",
			"access_token": jwtForTest(t, map[string]any{
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": "claim-account",
				},
			}),
		},
	})
	auth, err := ReadUsageAuth(filepath.Dir(authPath))
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccountID != "claim-account" || auth.AccessToken == "" {
		t.Fatalf("usage auth account = %q token empty = %t", auth.AccountID, auth.AccessToken == "")
	}
}

func TestReadUsageAuthRejectsAPIKeyProfiles(t *testing.T) {
	authPath := writeAuthForTest(t, map[string]any{
		"auth_mode": "apikey",
		"tokens":    map[string]any{"access_token": "synthetic"},
	})
	if _, err := ReadUsageAuth(filepath.Dir(authPath)); err == nil {
		t.Fatal("API key profile unexpectedly accepted for ChatGPT quota")
	}
}
