package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const unreadableAuthLabel = "unreadable-auth"

func (process *CodexProcess) InspectQuotaAuth(ctx context.Context, codexHome string) (profilemodel.QuotaAuthSummary, error) {
	if err := ctx.Err(); err != nil {
		return profilemodel.QuotaAuthSummary{}, err
	}
	modelProvider, err := process.InspectModelProvider(ctx, codexHome)
	if err != nil {
		if ctx.Err() != nil {
			return profilemodel.QuotaAuthSummary{}, ctx.Err()
		}
		return quotaAuthSummary("config-error", false), nil
	}
	if modelProvider != nil {
		return quotaAuthSummary("model-provider:"+modelProvider.ProviderID, false), nil
	}
	path := filepath.Join(codexHome, "auth.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return quotaAuthSummary("no-auth", false), nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxAuthFileSize {
		return quotaAuthSummary(unreadableAuthLabel, false), nil
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return quotaAuthSummary(unreadableAuthLabel, false), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return quotaAuthSummary(unreadableAuthLabel, false), nil
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxAuthFileSize+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(content) > maxAuthFileSize {
		clear(content)
		return quotaAuthSummary(unreadableAuthLabel, false), nil
	}
	defer clear(content)
	return summarizeQuotaAuth(content), nil
}

func summarizeQuotaAuth(content []byte) profilemodel.QuotaAuthSummary {
	var auth codexAuthFile
	if err := json.Unmarshal(content, &auth); err != nil {
		return quotaAuthSummary("invalid-auth", false)
	}
	defer clearAuthTokens(&auth)
	if strings.TrimSpace(auth.Tokens.AccessToken) != "" {
		return quotaAuthSummary("chatgpt", true)
	}
	mode := normalizeQuotaAuthMode(auth.AuthMode)
	if mode == "apikey" || strings.TrimSpace(auth.OpenAIAPIKey) != "" {
		return quotaAuthSummary("api-key", false)
	}
	if mode == "" {
		return quotaAuthSummary("auth-present", false)
	}
	return quotaAuthSummary(strings.ToLower(strings.TrimSpace(auth.AuthMode)), false)
}

func normalizeQuotaAuthMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	return value
}

func quotaAuthSummary(label string, compatible bool) profilemodel.QuotaAuthSummary {
	return profilemodel.QuotaAuthSummary{Label: label, Compatible: compatible}
}
