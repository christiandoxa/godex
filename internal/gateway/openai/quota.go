package openai

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const maxQuotaResponseBytes = 1 << 20

type quotaAuthReader interface {
	authReader
	ReadQuotaResidency(context.Context, string) (string, error)
}

type QuotaClient struct {
	client   *http.Client
	upstream *url.URL
	auth     quotaAuthReader
}

func NewQuotaClient(upstream string, client *http.Client, auth quotaAuthReader) (*QuotaClient, error) {
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
	return client.FetchAtPolicy(ctx, codexHome, upstream, false)
}

func (client *QuotaClient) FetchAtPolicy(ctx context.Context, codexHome, upstream string, noProxy bool) (quotamodel.Usage, error) {
	body, err := client.fetchBodyAtPolicy(ctx, codexHome, upstream, noProxy)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	usage, err := decodeQuotaUsage(body)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	return usage, nil
}

// FetchAuthenticatedAtPolicy observes quota over the actual request-bound
// credential, not over an in-memory/persisted quota snapshot. Re-read after
// success rejects a profile reauthenticated concurrently with the request.
// No raw credential is returned or persisted.
func (client *QuotaClient) FetchAuthenticatedAtPolicy(
	ctx context.Context, codexHome, upstream string, noProxy bool,
) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error) {
	body, requestAuth, startedAt, err := client.fetchBodyWithIdentity043714(ctx, codexHome, upstream, noProxy)
	if err != nil {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, err
	}
	usage, err := decodeQuotaUsage(body)
	if err != nil {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, err
	}
	original, ok := openAIAuthDigest043714(requestAuth)
	if !ok {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, errors.New("quota credential unavailable")
	}
	currentAuth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, err
	}
	current, currentOK := openAIAuthDigest043714(currentAuth)
	if !currentOK || subtle.ConstantTimeCompare(original[:], current[:]) != 1 {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, errors.New("quota credentials changed during authenticated observation")
	}
	return usage, quotamodel.AuthenticatedReceipt{CredentialDigest: original, StartedAt: startedAt, Confirmed: true}, nil
}

// CurrentCredentialDigest verifies the *currently configured* managed
// credential without contacting the provider. It lets a previously confirmed
// probe receipt be revoked if local credentials change before route selection.
func (client *QuotaClient) CurrentCredentialDigest(ctx context.Context, codexHome string) ([32]byte, error) {
	auth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return [32]byte{}, err
	}
	digest, ok := openAIAuthDigest043714(auth)
	if !ok {
		return [32]byte{}, errors.New("quota credential unavailable")
	}
	return digest, nil
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
	return client.fetchBodyAtPolicy(ctx, codexHome, upstream, false)
}

func (client *QuotaClient) fetchBodyAtPolicy(ctx context.Context, codexHome, upstream string, noProxy bool) ([]byte, error) {
	body, _, _, err := client.fetchBodyWithIdentity043714(ctx, codexHome, upstream, noProxy)
	return body, err
}

// fetchBodyWithIdentity043714 keeps the credential used for the final HTTP
// attempt alongside its reply. Prodex 0.437.14 retries an unauthorized quota
// request only after a changed on-disk credential or an explicit auth refresh.
// A failed attempt cannot become authenticated recovery evidence.
func (client *QuotaClient) fetchBodyWithIdentity043714(
	ctx context.Context, codexHome, upstream string, noProxy bool,
) ([]byte, proxymodel.Auth, time.Time, error) {
	auth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return nil, proxymodel.Auth{}, time.Time{}, err
	}
	usageURL, err := client.usageURL(upstream)
	if err != nil {
		return nil, proxymodel.Auth{}, time.Time{}, err
	}
	httpClient, err := client.clientForPolicy(noProxy)
	if err != nil {
		return nil, proxymodel.Auth{}, time.Time{}, err
	}
	send := func(identity proxymodel.Auth) (int, []byte, time.Time, error) {
		residency, err := client.auth.ReadQuotaResidency(ctx, codexHome)
		if err != nil {
			return 0, nil, time.Time{}, err
		}
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
		if requestErr != nil {
			return 0, nil, time.Time{}, fmt.Errorf("create quota request: %w", requestErr)
		}
		request.Header.Set("Authorization", "Bearer "+identity.AccessToken)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("originator", "codex_cli_rs")
		request.Header.Set("x-openai-codex-luna-reserve", "1")
		if residency != "" {
			request.Header.Set("x-openai-internal-codex-residency", residency)
		}
		if identity.AccountID != "" {
			request.Header.Set("ChatGPT-Account-Id", identity.AccountID)
		}
		startedAt := time.Now()
		response, sendErr := client.doWithClient(httpClient, request)
		if sendErr != nil {
			return 0, nil, time.Time{}, sendErr
		}
		defer response.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxQuotaResponseBytes+1))
		if readErr != nil {
			return 0, nil, time.Time{}, fmt.Errorf("read quota response: %w", readErr)
		}
		if len(body) > maxQuotaResponseBytes {
			return 0, nil, time.Time{}, errors.New("quota response exceeded safe size limit")
		}
		return response.StatusCode, body, startedAt, nil
	}
	status, body, startedAt, err := send(auth)
	if err != nil {
		return nil, proxymodel.Auth{}, time.Time{}, err
	}
	if status == http.StatusUnauthorized {
		// ReadAuth is the authoritative fresh on-disk identity; never replay
		// an identical bearer token without a real refresh result.
		if reloaded, reloadErr := client.auth.ReadAuth(ctx, codexHome); reloadErr == nil &&
			runtimeAuthChanged(auth, reloaded) && reloaded.AccessToken != "" {
			auth = reloaded
			status, body, startedAt, err = send(auth)
			if err != nil {
				return nil, proxymodel.Auth{}, time.Time{}, err
			}
		}
		if status == http.StatusUnauthorized {
			if refresher, ok := client.auth.(unauthorizedAuthRefresher); ok {
				if refreshed, refreshErr := refresher.RefreshUnauthorizedAuth(ctx, codexHome, auth); refreshErr == nil &&
					refreshed.AccessToken != "" {
					auth = refreshed
					status, body, startedAt, err = send(auth)
					if err != nil {
						return nil, proxymodel.Auth{}, time.Time{}, err
					}
				}
			}
		}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, proxymodel.Auth{}, time.Time{}, fmt.Errorf("quota endpoint returned HTTP %d", status)
	}
	return body, auth, startedAt, nil
}

func (client *QuotaClient) do(request *http.Request) (*http.Response, error) {
	return client.doWithClient(client.client, request)
}

func (client *QuotaClient) doWithClient(httpClient *http.Client, request *http.Request) (*http.Response, error) {
	response, err := httpClient.Do(request)
	if err == nil {
		return response, nil
	}
	if request.Context().Err() != nil {
		return nil, request.Context().Err()
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return nil, err
	}
	clone := request.Clone(request.Context())
	response, retryErr := httpClient.Do(clone)
	if retryErr != nil {
		return nil, fmt.Errorf("request quota endpoint: %w", retryErr)
	}
	return response, nil
}

func (client *QuotaClient) clientForPolicy(noProxy bool) (*http.Client, error) {
	if !noProxy {
		return client.client, nil
	}
	copy := *client.client
	transport, ok := copy.Transport.(*http.Transport)
	if !ok {
		return nil, errors.New("quota --no-proxy requires an HTTP transport")
	}
	transport = transport.Clone()
	transport.Proxy = nil
	copy.Transport = transport
	return &copy, nil
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
