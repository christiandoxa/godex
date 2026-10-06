package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	chatGPTAuthRefreshURL      = "https://auth.openai.com/oauth/token"
	chatGPTAuthRefreshClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	refreshTokenOverrideEnv    = "CODEX_REFRESH_TOKEN_URL_OVERRIDE"
	refreshResponseMaxBytes    = 1 << 20
)

type chatGPTRefreshWireResponse struct {
	IDToken      *string `json:"id_token"`
	AccessToken  *string `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
}

func (process *CodexProcess) RefreshUnauthorizedAuth(
	ctx context.Context,
	codexHome string,
	expected proxymodel.Auth,
) (proxymodel.Auth, error) {
	if err := ctx.Err(); err != nil {
		return proxymodel.Auth{}, err
	}
	codexHome = filepath.Clean(strings.TrimSpace(codexHome))
	if codexHome == "" || !filepath.IsAbs(codexHome) {
		return proxymodel.Auth{}, errors.New("Codex home must be an absolute path")
	}

	release, err := lockfile.Acquire(ctx, filepath.Join(codexHome, ".godex-auth-refresh.lock"))
	if err != nil {
		return proxymodel.Auth{}, fmt.Errorf("acquire auth refresh lock: %w", err)
	}
	defer release()

	current, err := ReadUsageAuth(codexHome)
	if err != nil {
		return proxymodel.Auth{}, err
	}
	currentProxy := usageAuthProxy(current)
	if runtimeAuthDiffers(expected, currentProxy) {
		return currentProxy, nil
	}
	refreshToken := strings.TrimSpace(current.RefreshToken)
	if refreshToken == "" {
		return proxymodel.Auth{}, errors.New("refresh token is missing from the stored auth secret")
	}

	refreshed, err := requestChatGPTAuthRefresh(ctx, refreshToken)
	if err != nil {
		return proxymodel.Auth{}, err
	}

	latest, err := ReadUsageAuth(codexHome)
	if err != nil {
		return proxymodel.Auth{}, err
	}
	if strings.TrimSpace(latest.RefreshToken) != refreshToken {
		return usageAuthProxy(latest), nil
	}

	authPath := filepath.Join(codexHome, "auth.json")
	content, err := readPrivateAuthFile(authPath)
	if err != nil {
		return proxymodel.Auth{}, err
	}
	defer clear(content)
	updated, err := ApplyChatGPTRefresh(content, refreshed, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return proxymodel.Auth{}, err
	}
	defer clear(updated)
	committed, err := fileutil.AtomicWrite(authPath, updated)
	if err != nil {
		return proxymodel.Auth{}, fmt.Errorf("commit refreshed auth: %w", err)
	}
	if !committed {
		return proxymodel.Auth{}, errors.New("refreshed auth was not committed")
	}
	final, err := ReadUsageAuth(codexHome)
	if err != nil {
		return proxymodel.Auth{}, err
	}
	return usageAuthProxy(final), nil
}

func usageAuthProxy(auth UsageAuth) proxymodel.Auth {
	return proxymodel.Auth{
		AccessToken: strings.TrimSpace(auth.AccessToken),
		AccountID:   strings.TrimSpace(auth.AccountID),
	}
}

func runtimeAuthDiffers(previous, current proxymodel.Auth) bool {
	return previous.AccessToken != current.AccessToken || previous.AccountID != current.AccountID
}

func requestChatGPTAuthRefresh(ctx context.Context, refreshToken string) (ChatGPTRefreshResponse, error) {
	endpoint := strings.TrimSpace(os.Getenv(refreshTokenOverrideEnv))
	if endpoint == "" {
		endpoint = chatGPTAuthRefreshURL
	}
	payload, err := json.Marshal(map[string]string{
		"client_id":     chatGPTAuthRefreshClientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	})
	if err != nil {
		return ChatGPTRefreshResponse{}, err
	}
	defer clear(payload)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ChatGPTRefreshResponse{}, fmt.Errorf("build ChatGPT auth refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("originator", "codex_cli_rs")

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return ChatGPTRefreshResponse{}, fmt.Errorf("request ChatGPT auth refresh: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, refreshResponseMaxBytes+1))
	if err != nil {
		return ChatGPTRefreshResponse{}, fmt.Errorf("read ChatGPT auth refresh response: %w", err)
	}
	defer clear(body)
	if len(body) > refreshResponseMaxBytes {
		return ChatGPTRefreshResponse{}, errors.New("ChatGPT auth refresh response exceeds safe size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ChatGPTRefreshResponse{}, fmt.Errorf("failed to refresh ChatGPT auth (HTTP %d)", response.StatusCode)
	}
	var wire chatGPTRefreshWireResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return ChatGPTRefreshResponse{}, errors.New("failed to parse auth refresh JSON")
	}
	if wire.IDToken == nil && wire.AccessToken == nil && wire.RefreshToken == nil {
		return ChatGPTRefreshResponse{}, errors.New("auth refresh JSON contained no token fields")
	}
	return ChatGPTRefreshResponse{
		IDToken: wire.IDToken, AccessToken: wire.AccessToken, RefreshToken: wire.RefreshToken,
	}, nil
}
