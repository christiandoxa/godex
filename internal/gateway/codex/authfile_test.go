package codex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadChatGPTIdentity(t *testing.T) {
	claims := map[string]any{
		"email": "Person@Example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-123",
		},
	}
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"id_token": jwtForTest(t, claims),
		},
	}
	path := writeAuthForTest(t, auth)

	identity, err := readChatGPTIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Email != "person@example.com" {
		t.Fatalf("email = %q", identity.Email)
	}
	if identity.ChatGPTAccountID != "account-123" {
		t.Fatalf("account ID = %q", identity.ChatGPTAccountID)
	}
}

func TestReadChatGPTIdentityPrefersTokenAccountID(t *testing.T) {
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"account_id":   "stored-account",
			"access_token": jwtForTest(t, map[string]any{"chatgpt_account_id": "claim-account"}),
		},
	}
	identity, err := readChatGPTIdentity(writeAuthForTest(t, auth))
	if err != nil {
		t.Fatal(err)
	}
	if identity.ChatGPTAccountID != "claim-account" {
		t.Fatalf("account ID = %q", identity.ChatGPTAccountID)
	}
}

func TestReadChatGPTIdentityReadsNamespacedAccessClaims(t *testing.T) {
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": jwtForTest(t, map[string]any{
				"https://api.openai.com/profile": map[string]any{
					"email": "Person@Example.com",
				},
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": "account-456",
				},
			}),
		},
	}
	identity, err := readChatGPTIdentity(writeAuthForTest(t, auth))
	if err != nil {
		t.Fatal(err)
	}
	if identity.Email != "person@example.com" || identity.ChatGPTAccountID != "account-456" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestReadChatGPTIdentityRejectsOtherAuthModes(t *testing.T) {
	auth := map[string]any{
		"auth_mode": "apikey",
		"tokens": map[string]any{
			"access_token": jwtForTest(t, map[string]any{"email": "person@example.com"}),
		},
	}
	if _, err := readChatGPTIdentity(writeAuthForTest(t, auth)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReadChatGPTIdentityRequiresChatGPTAuthMode(t *testing.T) {
	auth := map[string]any{
		"tokens": map[string]any{
			"account_id": "account-123",
		},
	}
	if _, err := readChatGPTIdentity(writeAuthForTest(t, auth)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReadChatGPTIdentityDoesNotLeakTokenInError(t *testing.T) {
	secret := "synthetic-access-token-that-must-not-appear"
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": secret,
		},
	}
	_, err := readChatGPTIdentity(writeAuthForTest(t, auth))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked token or was unexpectedly nil: %v", err)
	}
}

func TestReadChatGPTIdentityRejectsAuthSymlink(t *testing.T) {
	target := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"account_id": "account-123",
		},
	})
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readChatGPTIdentity(path); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestReadAccessToken(t *testing.T) {
	path := writeAuthForTest(t, map[string]any{
		"auth_mode": "chatgpt",
		"tokens":    map[string]any{"access_token": "synthetic-access-token"},
	})
	token, err := ReadAccessToken(path)
	if err != nil || token != "synthetic-access-token" {
		t.Fatalf("access token = %q, err = %v", token, err)
	}
}

func TestReadAccessTokenRejectsInvalidProfiles(t *testing.T) {
	for _, test := range []struct {
		name string
		auth map[string]any
	}{
		{name: "wrong auth mode", auth: map[string]any{"auth_mode": "apikey"}},
		{name: "missing token", auth: map[string]any{"auth_mode": "chatgpt"}},
		{name: "malformed JSON", auth: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			content := []byte("{")
			if test.auth != nil {
				var err error
				content, err = json.Marshal(test.auth)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadAccessToken(path); err == nil {
				t.Fatal("invalid auth profile unexpectedly accepted")
			}
		})
	}
}

func TestReadAccessTokenRejectsMissingAndOversizedFiles(t *testing.T) {
	if _, err := ReadAccessToken(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing auth profile unexpectedly accepted")
	}
	path := filepath.Join(t.TempDir(), "large-auth.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxAuthFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAccessToken(path); err == nil {
		t.Fatal("oversized auth profile unexpectedly accepted")
	}
}

func jwtForTest(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".synthetic"
}

func writeAuthForTest(t *testing.T, auth map[string]any) string {
	t.Helper()
	content, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
