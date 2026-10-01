package openai

import (
	"context"
	"encoding/json"
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
	parsed, err := parseQuotaUpstream(upstream)
	if err != nil {
		return nil, err
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
	return client.FetchAt(ctx, codexHome, "")
}

func (client *QuotaClient) FetchAt(ctx context.Context, codexHome, upstream string) (quotamodel.Usage, error) {
	body, err := client.fetchBodyAt(ctx, codexHome, upstream)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	usage, err := decodeQuotaUsage(body)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	return usage, nil
}

func (client *QuotaClient) FetchRaw(ctx context.Context, codexHome string) ([]byte, error) {
	return client.FetchRawAt(ctx, codexHome, "")
}

func (client *QuotaClient) FetchRawAt(ctx context.Context, codexHome, upstream string) ([]byte, error) {
	body, err := client.fetchBodyAt(ctx, codexHome, upstream)
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, errors.New("decode quota response")
	}
	return body, nil
}

func (client *QuotaClient) fetchBodyAt(ctx context.Context, codexHome, upstream string) ([]byte, error) {
	auth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return nil, err
	}
	usageURL, err := client.usageURL(upstream)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create quota request: %w", err)
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
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxQuotaResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read quota response: %w", err)
	}
	if len(body) > maxQuotaResponseBytes {
		return nil, errors.New("quota response exceeded safe size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("quota endpoint returned HTTP %d", response.StatusCode)
	}
	return body, nil
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

func (client *QuotaClient) usageURL(override string) (string, error) {
	target := client.upstream
	if strings.TrimSpace(override) != "" {
		parsed, err := parseQuotaUpstream(override)
		if err != nil {
			return "", err
		}
		target = parsed
	}
	copy := *target
	base := strings.TrimRight(copy.Path, "/")
	if strings.Contains(base, "/backend-api") {
		copy.Path = base + "/wham/usage"
	} else {
		copy.Path = base + "/api/codex/usage"
	}
	copy.RawPath = ""
	return copy.String(), nil
}

func parseQuotaUpstream(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("quota upstream URL must be an http(s) URL without credentials or query data")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("quota upstream URL must use http or https")
	}
	return parsed, nil
}
