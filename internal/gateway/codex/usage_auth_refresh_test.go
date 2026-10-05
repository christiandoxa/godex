package codex

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestUsageAuthMatchesProdexRefreshMetadata(t *testing.T) {
	jwt := jwtForTest(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": " acct_jwt "},
		"exp":                         int64(200),
	})
	content, err := json.Marshal(map[string]any{
		"tokens": map[string]any{
			"access_token":  jwt,
			"account_id":    "acct_stored",
			"refresh_token": " refresh-1 ",
		},
		"last_refresh": "1970-01-01T00:00:05Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := decodeUsageAuth(content)
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != jwt || auth.AccountID != "acct_jwt" || auth.RefreshToken != "refresh-1" ||
		auth.ExpiresAt == nil || *auth.ExpiresAt != 200 || auth.LastRefresh == nil || *auth.LastRefresh != 5 {
		t.Fatalf("usage auth = %#v", auth)
	}
}

func TestUsageAuthAcceptsPersonalAccessTokenAndOpaqueAccessToken(t *testing.T) {
	content := []byte("{\"auth_mode\":\"personalAccessToken\",\"tokens\":{\"access_token\":\" at-v2-opaque-token \",\"account_id\":\" acct_stored \",\"refresh_token\":\" refresh-2 \"}}")
	auth, err := decodeUsageAuth(content)
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "at-v2-opaque-token" || auth.AccountID != "acct_stored" ||
		auth.RefreshToken != "refresh-2" || auth.ExpiresAt != nil || auth.LastRefresh != nil {
		t.Fatalf("personal access token auth = %#v", auth)
	}
}

func TestUsageAuthRejectsProviderManagedAPIKeys(t *testing.T) {
	tests := map[string]string{
		"openai key":   "{\"auth_mode\":\"chatgpt\",\"OPENAI_API_KEY\":\"sk-synthetic\",\"tokens\":{\"access_token\":\"opaque\"}}",
		"bedrock key":  "{\"auth_mode\":\"chatgpt\",\"bedrock_api_key\":{\"api_key\":\"bedrock-synthetic\",\"region\":\"us-east-1\"},\"tokens\":{\"access_token\":\"opaque\"}}",
		"api mode":     "{\"auth_mode\":\"API-Key\",\"tokens\":{\"access_token\":\"opaque\"}}",
		"bedrock mode": "{\"auth_mode\":\" Bedrock_API-Key \",\"tokens\":{\"access_token\":\"opaque\"}}",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeUsageAuth([]byte(content)); err == nil {
				t.Fatal("provider-managed API-key auth unexpectedly accepted")
			}
		})
	}
}

func TestUsageAuthIgnoresNonCanonicalOrNonIntegerJWTExpiry(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte("{\"alg\":\"none\"}"))
	payload := base64.RawURLEncoding.EncodeToString([]byte("{\"https://api.openai.com/auth\":{\"chatgpt_account_id\":\"jwt\"},\"exp\":200}"))
	padded := header + "." + payload + "=.sig"
	auth, err := decodeUsageAuth([]byte("{\"tokens\":{\"access_token\":\"" + padded + "\",\"account_id\":\"stored\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccountID != "stored" || auth.ExpiresAt != nil {
		t.Fatalf("noncanonical JWT influenced auth = %#v", auth)
	}

	fractional := jwtForTest(t, map[string]any{"exp": 200.5})
	auth, err = decodeUsageAuth([]byte("{\"tokens\":{\"access_token\":\"" + fractional + "\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	if auth.ExpiresAt != nil {
		t.Fatalf("fractional JWT exp accepted: %#v", auth)
	}
}

func TestUsageAuthNeedsProactiveRefreshMatchesProdexPolicy(t *testing.T) {
	exp := int64(110)
	if !UsageAuthNeedsProactiveRefresh(UsageAuth{ExpiresAt: &exp}, 100, 10, 8) {
		t.Fatal("expiry boundary did not refresh")
	}
	exp = 111
	if UsageAuthNeedsProactiveRefresh(UsageAuth{ExpiresAt: &exp}, 100, 10, 8) {
		t.Fatal("expiry above boundary refreshed")
	}
	last := int64(0)
	if !UsageAuthNeedsProactiveRefresh(UsageAuth{LastRefresh: &last}, 8*86400, 10, 8) {
		t.Fatal("refresh interval boundary did not refresh")
	}
	if UsageAuthNeedsProactiveRefresh(UsageAuth{}, 8*86400, 10, 8) {
		t.Fatal("missing refresh metadata refreshed")
	}
	exp = math.MaxInt64
	if !UsageAuthNeedsProactiveRefresh(UsageAuth{ExpiresAt: &exp}, math.MaxInt64-5, 10, 8) {
		t.Fatal("saturating expiry threshold did not refresh")
	}
}

func TestApplyChatGPTRefreshMatchesProdexOptionalFieldSemantics(t *testing.T) {
	access := jwtForTest(t, map[string]any{
		"https://api.openai.com/auth.chatgpt_account_id": "acct_refreshed",
	})
	id := "id"
	refresh := "new"
	input := []byte("{\"auth_mode\":\"chatgpt\",\"tokens\":{\"refresh_token\":\"old\",\"id_token\":\"old-id\",\"access_token\":\"old-access\",\"account_id\":\"old-account\"},\"future\":{\"keep\":true}}")
	output, err := ApplyChatGPTRefresh(
		input,
		ChatGPTRefreshResponse{IDToken: &id, AccessToken: &access, RefreshToken: &refresh},
		"2026-04-30T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	tokens := got["tokens"].(map[string]any)
	if tokens["id_token"] != "id" || tokens["access_token"] != access || tokens["account_id"] != "acct_refreshed" ||
		tokens["refresh_token"] != "new" || got["last_refresh"] != "2026-04-30T00:00:00Z" {
		t.Fatalf("refreshed auth = %#v", got)
	}
	if got["future"].(map[string]any)["keep"] != true {
		t.Fatalf("future auth field lost: %#v", got)
	}
}

func TestApplyChatGPTRefreshPreservesMissingFieldsAndCreatesTokens(t *testing.T) {
	empty := ""
	input := []byte("{\"auth_mode\":\"personalAccessToken\",\"tokens\":{\"id_token\":\"old-id\",\"access_token\":\"old-access\",\"refresh_token\":\"old-refresh\",\"account_id\":\"old-account\"}}")
	output, err := ApplyChatGPTRefresh(
		input,
		ChatGPTRefreshResponse{AccessToken: &empty},
		"2026-05-01T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	tokens := got["tokens"].(map[string]any)
	if tokens["access_token"] != "" || tokens["id_token"] != "old-id" || tokens["refresh_token"] != "old-refresh" ||
		tokens["account_id"] != "old-account" {
		t.Fatalf("optional refresh semantics = %#v", tokens)
	}

	refresh := "created"
	output, err = ApplyChatGPTRefresh([]byte("{\"auth_mode\":\"chatgpt\"}"), ChatGPTRefreshResponse{RefreshToken: &refresh}, "2026-05-02T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "\"refresh_token\":\"created\"") {
		t.Fatalf("tokens object was not created: %s", output)
	}
}

func TestApplyChatGPTRefreshRejectsInvalidStoredShapes(t *testing.T) {
	refresh := "new"
	for _, input := range []string{"[]", "{\"tokens\":\"not-an-object\"}"} {
		if _, err := ApplyChatGPTRefresh([]byte(input), ChatGPTRefreshResponse{RefreshToken: &refresh}, "2026-05-02T00:00:00Z"); err == nil {
			t.Fatalf("invalid auth shape accepted: %s", input)
		}
	}
}

func TestUsageAuthJWTAccountPrecedenceMatchesProdex(t *testing.T) {
	jwt := jwtForTest(t, map[string]any{
		"https://api.openai.com/auth":                    map[string]any{"chatgpt_account_id": " nested "},
		"https://api.openai.com/auth.chatgpt_account_id": "dotted",
		"chatgpt_account_id":                             "direct",
	})
	auth, err := decodeUsageAuth([]byte(`{"tokens":{"access_token":"` + jwt + `","account_id":"stored"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccountID != "nested" {
		t.Fatalf("JWT account precedence = %q", auth.AccountID)
	}
}

func TestUsageAuthAcceptsMissingAuthModeWhenTokensAreChatGPTCompatible(t *testing.T) {
	auth, err := decodeUsageAuth([]byte(`{"tokens":{"access_token":"opaque","account_id":"stored"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken != "opaque" || auth.AccountID != "stored" {
		t.Fatalf("missing-mode auth = %#v", auth)
	}
}

func TestApplyChatGPTRefreshPreservesUnknownJSONNumbersAndTextBytes(t *testing.T) {
	response := ChatGPTRefreshResponse{RefreshToken: new(string)}
	*response.RefreshToken = "next"
	output, err := ApplyChatGPTRefresh(
		[]byte(`{"tokens":{"access_token":"old"},"future":{"big":9007199254740993,"text":"<x>&"}}`),
		response,
		"2026-05-02T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if !strings.Contains(text, `"big":9007199254740993`) || !strings.Contains(text, `"text":"<x>&"`) ||
		strings.Contains(text, `\u003c`) || strings.Contains(text, `\u0026`) {
		t.Fatalf("unknown auth JSON changed: %s", output)
	}
}

func TestUsageAuthLastRefreshParsesRFC3339Offset(t *testing.T) {
	content := []byte(`{"tokens":{"access_token":"opaque"},"last_refresh":"1970-01-01T01:00:05+01:00"}`)
	auth, err := decodeUsageAuth(content)
	if err != nil {
		t.Fatal(err)
	}
	if auth.LastRefresh == nil || *auth.LastRefresh != 5 {
		t.Fatalf("last refresh = %#v", auth.LastRefresh)
	}
}
