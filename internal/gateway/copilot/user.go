package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const userInfoMaxBytes = 1 << 20
const invalidCopilotHost = "invalid Copilot host"

type userInfo struct {
	Login         *string        `json:"login"`
	AccessTypeSKU *string        `json:"access_type_sku"`
	CopilotPlan   *string        `json:"copilot_plan"`
	Endpoints     *userEndpoints `json:"endpoints"`
}

type userEndpoints struct {
	API *string `json:"api"`
}

func defaultHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (source *Source) fetchUserInfo(ctx context.Context, host, token string) (userInfo, error) {
	origin, err := copilotUserAPIOrigin(host)
	if err != nil {
		return userInfo{}, err
	}
	target := strings.TrimRight(origin, "/") + "/copilot_internal/user"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return userInfo{}, fmt.Errorf("create Copilot account request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "godex")
	response, err := source.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return userInfo{}, ctx.Err()
		}
		return userInfo{}, errors.New("failed to query Copilot account")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, userInfoMaxBytes+1))
	if err != nil || len(body) > userInfoMaxBytes {
		return userInfo{}, errors.New("failed to read Copilot account response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return userInfo{}, fmt.Errorf("Copilot account query failed (HTTP %d)", response.StatusCode)
	}
	var info userInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return userInfo{}, errors.New("failed to parse Copilot account response")
	}
	return info, nil
}

func copilotUserAPIOrigin(host string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(host), "/")
	if trimmed == "" {
		return "", errors.New(invalidCopilotHost)
	}
	scheme, rest := "https", trimmed
	if current, suffix, found := strings.Cut(trimmed, "://"); found {
		if current == "" || suffix == "" {
			return "", errors.New(invalidCopilotHost)
		}
		scheme, rest = current, suffix
	}
	authority := rest
	if index := strings.IndexAny(authority, "/?#"); index >= 0 {
		authority = authority[:index]
	}
	if authority == "" {
		return "", errors.New(invalidCopilotHost)
	}
	hostname, explicitPort, ok := authorityHostAndPort(authority)
	if !ok {
		return "", errors.New(invalidCopilotHost)
	}
	isLocal := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if !explicitPort && !isLocal && !strings.HasPrefix(hostname, "api.") {
		authority = "api." + authority
	}
	return scheme + "://" + authority, nil
}

func authorityHostAndPort(authority string) (string, bool, bool) {
	if strings.HasPrefix(authority, "[") {
		closing := strings.Index(authority, "]")
		if closing <= 1 {
			return "", false, false
		}
		after := authority[closing+1:]
		return authority[1:closing], strings.HasPrefix(after, ":"), true
	}
	colon := strings.LastIndex(authority, ":")
	if colon > 0 && colon < len(authority)-1 {
		port := authority[colon+1:]
		if allDigits(port) {
			return authority[:colon], true, true
		}
	}
	return authority, false, true
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func defaultCopilotAPIURL(host string) string {
	normalized := strings.TrimRight(strings.TrimSpace(host), "/")
	if strings.EqualFold(normalized, "https://github.com") || strings.EqualFold(normalized, "http://github.com") || strings.EqualFold(normalized, "github.com") {
		return "https://api.githubcopilot.com"
	}
	fallback := strings.TrimPrefix(strings.TrimPrefix(normalized, "https://"), "http://")
	if subdomain, found := strings.CutSuffix(fallback, ".ghe.com"); found {
		return "https://copilot-api." + subdomain + ".ghe.com"
	}
	return "https://api." + fallback
}
