package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	virtualConnectTimeout   = 5 * time.Second
	virtualRequestTimeout   = 10 * time.Second
	virtualBodyMaxBytes     = 4 << 20
	defaultDeepSeekURL      = "https://api.deepseek.com"
	virtualProviderDeepSeek = "deepseek"
	virtualProviderLocal    = "local"
	virtualProviderAgy      = "agy"
)

type Virtual struct {
	client *http.Client
	getenv func(string) string
	run    commandRunner
}

func NewVirtual(client *http.Client) *Virtual {
	return &Virtual{client: cloneVirtualClient(client), getenv: os.Getenv, run: runVirtualCommand}
}

func (virtual *Virtual) Collect(ctx context.Context, providerFilter, baseURL string) []quotamodel.VirtualResult {
	switch strings.ToLower(strings.TrimSpace(providerFilter)) {
	case virtualProviderDeepSeek:
		return virtual.collectDeepSeek(ctx, baseURL)
	case virtualProviderLocal:
		return []quotamodel.VirtualResult{virtual.collectLocal(ctx, baseURL)}
	case virtualProviderAgy:
		return virtual.collectAgy(ctx)
	default:
		return nil
	}
}

func (virtual *Virtual) collectDeepSeek(ctx context.Context, explicitBaseURL string) []quotamodel.VirtualResult {
	keys := virtual.deepSeekKeys()
	if len(keys) == 0 {
		return []quotamodel.VirtualResult{{
			Name: virtualProviderDeepSeek, Provider: virtualProviderDeepSeek, Auth: "deepseek-key",
			Err: errors.New("DeepSeek quota requires DEEPSEEK_API_KEY or DEEPSEEK_API_KEYS"),
		}}
	}
	results := make([]quotamodel.VirtualResult, 0, len(keys))
	for index, key := range keys {
		name := virtualProviderDeepSeek
		if index > 0 {
			name = fmt.Sprintf("deepseek-%d", index+1)
		}
		info, err := virtual.fetchDeepSeek(ctx, key, explicitBaseURL)
		result := quotamodel.VirtualResult{Name: name, Provider: "deepseek", Auth: "deepseek-key", Err: err}
		if err == nil {
			result.External = &info
		}
		results = append(results, result)
	}
	return results
}

func (virtual *Virtual) deepSeekKeys() []string {
	if value := virtual.getenv("DEEPSEEK_API_KEYS"); value != "" {
		if keys := splitProviderKeys(value); len(keys) > 0 {
			return keys
		}
	}
	if key := strings.TrimSpace(virtual.getenv("DEEPSEEK_API_KEY")); key != "" {
		return []string{key}
	}
	return nil
}

func splitProviderKeys(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';' || current == '\n'
	})
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if key := strings.TrimSpace(part); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func (virtual *Virtual) fetchDeepSeek(ctx context.Context, key, explicitBaseURL string) (quotamodel.ExternalInfo, error) {
	base, err := virtual.deepSeekBaseURL(explicitBaseURL)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/user/balance", nil)
	if err != nil {
		return quotamodel.ExternalInfo{}, errors.New("create DeepSeek balance request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	body, err := virtual.doJSON(request, "DeepSeek balance")
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	return decodeDeepSeekInfo(body)
}

func (virtual *Virtual) deepSeekBaseURL(explicit string) (string, error) {
	base := strings.TrimSpace(explicit)
	if base == "" {
		base = strings.TrimSpace(virtual.getenv("GODEX_DEEPSEEK_BASE_URL"))
	}
	if base == "" {
		base = strings.TrimSpace(virtual.getenv("PRODEX_DEEPSEEK_BASE_URL"))
	}
	if base == "" {
		base = strings.TrimSpace(virtual.getenv("DEEPSEEK_BASE_URL"))
	}
	if base == "" {
		base = defaultDeepSeekURL
	}
	base = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	parsed, err := validateVirtualHTTPURL(base)
	if err != nil {
		return "", fmt.Errorf("invalid DeepSeek base URL: %w", err)
	}
	return parsed.String(), nil
}

func decodeDeepSeekInfo(body []byte) (quotamodel.ExternalInfo, error) {
	var value struct {
		Available bool `json:"is_available"`
		Balances  []struct {
			Currency string `json:"currency"`
			Total    string `json:"total_balance"`
			Granted  string `json:"granted_balance"`
			ToppedUp string `json:"topped_up_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return quotamodel.ExternalInfo{}, errors.New("failed to parse DeepSeek balance response")
	}
	parts := make([]string, 0, len(value.Balances))
	details := make([]quotamodel.ExternalDetail, 0, len(value.Balances))
	for _, balance := range value.Balances {
		currency := valueOr(balance.Currency, "balance")
		total := valueOr(balance.Total, "-")
		granted := valueOr(balance.Granted, "-")
		topped := valueOr(balance.ToppedUp, "-")
		parts = append(parts, currency+" "+total)
		details = append(details, quotamodel.ExternalDetail{
			Label: currency + " balance",
			Value: fmt.Sprintf("total %s; granted %s; topped up %s", total, granted, topped),
		})
	}
	main := "balance unavailable"
	if len(parts) > 0 {
		main = strings.Join(parts, " | ")
	}
	available := value.Available
	status := "Blocked"
	if value.Available {
		status = "Ready"
	}
	return quotamodel.ExternalInfo{
		Provider: "DeepSeek", Plan: "api-key", Status: status, Main: main,
		Available: &available, Details: details,
	}, nil
}

func (virtual *Virtual) collectLocal(ctx context.Context, baseURL string) quotamodel.VirtualResult {
	result := quotamodel.VirtualResult{Name: virtualProviderLocal, Provider: virtualProviderLocal, Auth: virtualProviderLocal}
	if strings.TrimSpace(baseURL) == "" {
		result.Err = errors.New("local quota view requires --base-url pointing at the OpenAI-compatible server")
		return result
	}
	info, err := virtual.fetchLocal(ctx, baseURL)
	result.Err = err
	if err == nil {
		result.External = &info
	}
	return result
}

func (virtual *Virtual) fetchLocal(ctx context.Context, baseURL string) (quotamodel.ExternalInfo, error) {
	modelsURL, err := localModelsURL(baseURL)
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return quotamodel.ExternalInfo{}, errors.New("create local OpenAI-compatible models request")
	}
	request.Header.Set("Accept", "application/json")
	if key := virtual.localAPIKey(); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	body, err := virtual.doJSON(request, "local OpenAI-compatible models")
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	var value struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return quotamodel.ExternalInfo{}, errors.New("failed to parse local models response")
	}
	main := "models endpoint reachable"
	if value.Data != nil {
		main = fmt.Sprintf("%d model(s)", len(value.Data))
	}
	available := true
	return quotamodel.ExternalInfo{
		Provider: "Local OpenAI-compatible", Account: strings.TrimSpace(baseURL),
		Plan: "local", Status: "Reachable", Main: main, Available: &available,
		Details: []quotamodel.ExternalDetail{{Label: "Models URL", Value: modelsURL}},
	}, nil
}

func localModelsURL(baseURL string) (string, error) {
	parsed, err := validateVirtualHTTPURL(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid local base URL %q", strings.TrimSpace(baseURL))
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/v1/models"
	case !strings.HasSuffix(path, "/models"):
		parsed.Path = path + "/models"
	default:
		parsed.Path = path
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (virtual *Virtual) localAPIKey() string {
	for _, name := range []string{"PRODEX_LOCAL_API_KEY", "OPENAI_API_KEY"} {
		if value := strings.TrimSpace(virtual.getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func (virtual *Virtual) doJSON(request *http.Request, label string) ([]byte, error) {
	response, err := virtual.client.Do(request)
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		return nil, fmt.Errorf("failed to fetch %s", label)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, virtualBodyMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response", label)
	}
	if len(body) > virtualBodyMaxBytes {
		return nil, fmt.Errorf("%s response exceeded safe size limit", label)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s request failed (HTTP %d)", label, response.StatusCode)
	}
	return body, nil
}

func validateVirtualHTTPURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("URL must be credential-free http(s) without query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL must use http or https")
	}
	return parsed, nil
}

func cloneVirtualClient(client *http.Client) *http.Client {
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: virtualConnectTimeout, KeepAlive: 30 * time.Second}).DialContext
		client = &http.Client{Transport: transport, Timeout: virtualRequestTimeout}
	} else {
		copy := *client
		if copy.Timeout == 0 {
			copy.Timeout = virtualRequestTimeout
		}
		client = &copy
	}
	return client
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
