package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const maxQuotaResponseBytes = 1 << 20

type QuotaClient struct {
	client   *http.Client
	upstream *url.URL
	auth     authReader
}

func NewQuotaClient(upstream string, client *http.Client, auth authReader) (*QuotaClient, error) {
	if strings.TrimSpace(upstream) == "" {
		upstream = DefaultUpstreamURL
	}
	parsed, err := url.Parse(upstream)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("quota upstream URL must be an http(s) URL without credentials or query data")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("quota upstream URL must use http or https")
	}
	quotaClient := cloneHTTPClient(client)
	if quotaClient.Timeout == 0 {
		quotaClient.Timeout = 20 * time.Second
	}
	if auth == nil {
		return nil, errors.New("quota authentication reader is required")
	}
	return &QuotaClient{client: quotaClient, upstream: parsed, auth: auth}, nil
}

func (client *QuotaClient) Fetch(ctx context.Context, codexHome string) (quotamodel.Usage, error) {
	auth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.usageURL(), nil)
	if err != nil {
		return quotamodel.Usage{}, fmt.Errorf("create quota request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("x-openai-codex-luna-reserve", "1")
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}

	response, err := client.do(request)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxQuotaResponseBytes+1))
	if err != nil {
		return quotamodel.Usage{}, fmt.Errorf("read quota response: %w", err)
	}
	if len(body) > maxQuotaResponseBytes {
		return quotamodel.Usage{}, errors.New("quota response exceeded safe size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return quotamodel.Usage{}, fmt.Errorf("quota endpoint returned HTTP %d", response.StatusCode)
	}
	usage, err := decodeQuotaUsage(body)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	return usage, nil
}

func (client *QuotaClient) do(request *http.Request) (*http.Response, error) {
	response, err := client.client.Do(request)
	if err == nil {
		return response, nil
	}
	if request.Context().Err() != nil {
		return nil, request.Context().Err()
	}
	clone := request.Clone(request.Context())
	response, retryErr := client.client.Do(clone)
	if retryErr != nil {
		return nil, fmt.Errorf("request quota endpoint: %w", retryErr)
	}
	return response, nil
}

func (client *QuotaClient) usageURL() string {
	target := *client.upstream
	base := strings.TrimRight(target.Path, "/")
	if strings.Contains(base, "/backend-api") {
		target.Path = base + "/wham/usage"
	} else {
		target.Path = base + "/api/codex/usage"
	}
	target.RawPath = ""
	return target.String()
}
