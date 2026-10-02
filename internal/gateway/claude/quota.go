package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const anthropicQuotaBodyMaxBytes = 4 << 20

type quotaCredential struct {
	account    string
	authMethod string
	expiresAt  *int64
}

func (source *Source) FetchQuota(ctx context.Context, target profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	credential, err := source.quotaCredential(ctx, target.CodexHome)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	account := credential.account
	if account == "" {
		account = quotaSnapshotValue(target.ProviderConfig.Account)
	}
	authMethod := credential.authMethod
	if authMethod == "" {
		authMethod = quotaSnapshotValue(target.ProviderConfig.AuthMethod)
	}
	details := make([]quotamodel.ExternalDetail, 0, 6)
	if credential.expiresAt != nil {
		details = append(details, quotamodel.ExternalDetail{
			Label: "OAuth expires", Value: formatClaudeQuotaExpiry(*credential.expiresAt),
		})
	}
	if adminKey := source.anthropicAdminKey(); adminKey != "" {
		main, rateDetails, err := source.fetchAnthropicRateLimits(ctx, adminKey)
		if err == nil {
			details = append(details, rateDetails...)
			available := true
			return quotamodel.ExternalInfo{
				Provider: "Anthropic Claude", Account: account, Plan: authMethod,
				Status: "Ready", Main: main, Available: &available, Details: details,
			}, nil
		}
		details = append(details, quotamodel.ExternalDetail{Label: "Admin API", Value: "error: " + firstClaudeQuotaErrorLine(err)})
	} else {
		details = append(details, quotamodel.ExternalDetail{
			Label: "Admin API", Value: "set ANTHROPIC_ADMIN_KEY for configured rate limits",
		})
	}
	available := true
	return quotamodel.ExternalInfo{
		Provider: "Anthropic Claude", Account: account, Plan: authMethod,
		Status: "Ready (OAuth)", Main: "rate limits require admin key",
		Available: &available, Details: details,
	}, nil
}

func (source *Source) quotaCredential(ctx context.Context, home string) (quotaCredential, error) {
	if _, err := source.RuntimeOAuth(ctx, home); err != nil {
		return quotaCredential{}, err
	}
	text, err := readExternalCredential(home)
	if err != nil {
		return quotaCredential{}, err
	}
	var file credentialsFile
	if err := json.Unmarshal([]byte(text), &file); err != nil {
		return quotaCredential{}, errors.New("invalid Claude credentials JSON")
	}
	if file.ClaudeAIOAuth != nil {
		return quotaCredential{
			account:    strings.TrimSpace(file.ClaudeAIOAuth.Email),
			authMethod: authMethodLabel(file.ClaudeAIOAuth.Subscription),
			expiresAt:  file.ClaudeAIOAuth.ExpiresAt,
		}, nil
	}
	return quotaCredential{
		account: strings.TrimSpace(file.Email), authMethod: authMethodLabel(file.Subscription), expiresAt: file.ExpiresAt,
	}, nil
}

func (source *Source) anthropicAdminKey() string {
	for _, name := range []string{"ANTHROPIC_ADMIN_KEY", "ANTHROPIC_ADMIN_API_KEY"} {
		if value := strings.TrimSpace(source.getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func (source *Source) anthropicAdminBaseURL() (string, error) {
	base := strings.TrimSpace(source.getenv("PRODEX_ANTHROPIC_ADMIN_BASE_URL"))
	if base == "" {
		base = strings.TrimSpace(source.getenv("ANTHROPIC_BASE_URL"))
	}
	if base == "" {
		base = "https://api.anthropic.com"
	}
	parsed, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Anthropic admin base URL must be a credential-free http(s) URL without query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("Anthropic admin base URL must use http or https")
	}
	return parsed.String(), nil
}

func (source *Source) fetchAnthropicRateLimits(ctx context.Context, adminKey string) (string, []quotamodel.ExternalDetail, error) {
	base, err := source.anthropicAdminBaseURL()
	if err != nil {
		return "", nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v1/organizations/rate_limits", nil)
	if err != nil {
		return "", nil, errors.New("create Anthropic rate limit request")
	}
	request.Header.Set("anthropic-version", anthropicAPIVersion)
	request.Header.Set("accept", "application/json")
	request.Header.Set("x-api-key", adminKey)
	client := cloneAnthropicQuotaClient()
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, errors.New("failed to fetch Anthropic rate limits")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, anthropicQuotaBodyMaxBytes+1))
	if err != nil || len(body) > anthropicQuotaBodyMaxBytes {
		return "", nil, errors.New("failed to read Anthropic rate limit response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, fmt.Errorf("Anthropic rate limit request failed (HTTP %d)", response.StatusCode)
	}
	return anthropicRateLimitsSummary(body)
}

func anthropicRateLimitsSummary(body []byte) (string, []quotamodel.ExternalDetail, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value struct {
		Data []map[string]any `json:"data"`
	}
	if err := decoder.Decode(&value); err != nil {
		return "", nil, errors.New("failed to parse Anthropic rate limit response")
	}
	modelGroups := 0
	for _, entry := range value.Data {
		if group, _ := entry["group_type"].(string); group == "model_group" {
			modelGroups++
		}
	}
	details := []quotamodel.ExternalDetail{
		{Label: "Rate groups", Value: fmt.Sprintf("%d", len(value.Data))},
		{Label: "Model groups", Value: fmt.Sprintf("%d", modelGroups)},
	}
	for _, entry := range value.Data[:min(3, len(value.Data))] {
		if detail, ok := anthropicRateLimitDetail(entry); ok {
			details = append(details, detail)
		}
	}
	main := fmt.Sprintf("%d rate group(s)", len(value.Data))
	if modelGroups > 0 {
		main = fmt.Sprintf("%d model rate group(s)", modelGroups)
	}
	return main, details, nil
}

func anthropicRateLimitDetail(entry map[string]any) (quotamodel.ExternalDetail, bool) {
	limits, ok := entry["limits"].([]any)
	if !ok {
		return quotamodel.ExternalDetail{}, false
	}
	return quotamodel.ExternalDetail{
		Label: anthropicRateLimitLabel(entry),
		Value: strings.Join(anthropicRateLimitParts(limits), ", "),
	}, true
}

func anthropicRateLimitLabel(entry map[string]any) string {
	group, _ := entry["group_type"].(string)
	if group == "" {
		group = "rate_limit"
	}
	if group != "model_group" {
		return group
	}
	models, ok := entry["models"].([]any)
	if !ok || len(models) == 0 {
		return group
	}
	model, ok := models[0].(string)
	if !ok || strings.TrimSpace(model) == "" {
		return group
	}
	return model
}

func anthropicRateLimitParts(limits []any) []string {
	parts := make([]string, 0, len(limits))
	for _, raw := range limits {
		limit, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := limit["type"].(string)
		value, exists := limit["value"]
		if kind == "" || !exists {
			continue
		}
		parts = append(parts, kind+"="+claudeQuotaScalar(value))
	}
	return parts
}

func claudeQuotaScalar(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, err := json.Marshal(value)
	if err != nil {
		return "-"
	}
	return string(content)
}

func quotaSnapshotValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func formatClaudeQuotaExpiry(expiresAtMS int64) string {
	if expiresAtMS <= 0 {
		return "unknown"
	}
	return time.Unix(expiresAtMS/1000, 0).Local().Format("2006-01-02 15:04:05")
}

func firstClaudeQuotaErrorLine(err error) string {
	if err == nil {
		return "unknown error"
	}
	line := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	if line == "" {
		return "unknown error"
	}
	return line
}

func cloneAnthropicQuotaClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	return &http.Client{
		Transport: transport, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
