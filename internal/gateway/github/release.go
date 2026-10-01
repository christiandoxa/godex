package github

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultRepository = "christiandoxa/godex"

type EnvironmentReleaseClient struct {
	userAgent string
	client    *http.Client
}

func NewEnvironmentReleaseClient(userAgent string, client *http.Client) *EnvironmentReleaseClient {
	return &EnvironmentReleaseClient{userAgent: userAgent, client: client}
}

func (client *EnvironmentReleaseClient) LatestVersion(ctx context.Context) (string, error) {
	repository := strings.TrimSpace(os.Getenv("GODEX_REPOSITORY"))
	delegate, err := NewReleaseClient("", repository, client.userAgent, client.client)
	if err != nil {
		return "", err
	}
	return delegate.LatestVersion(ctx)
}

type ReleaseClient struct {
	client    *http.Client
	latestURL string
	userAgent string
	repoPath  string
	host      string
}

func NewReleaseClient(latestURL, repository, userAgent string, client *http.Client) (*ReleaseClient, error) {
	if strings.TrimSpace(repository) == "" {
		repository = defaultRepository
	}
	if !validRepository(repository) {
		return nil, errors.New("Godex repository must be owner/name without URL syntax")
	}
	if latestURL == "" {
		latestURL = "https://github.com/" + repository + "/releases/latest"
	}
	parsed, err := url.Parse(latestURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("latest release URL must be credential-free http(s)")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("latest release URL must use http or https")
	}
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: 800 * time.Millisecond}).DialContext
		client = &http.Client{Transport: transport, Timeout: 1200 * time.Millisecond}
	} else {
		copy := *client
		if copy.Timeout == 0 {
			copy.Timeout = 1200 * time.Millisecond
		}
		client = &copy
	}
	return &ReleaseClient{client: client, latestURL: parsed.String(), userAgent: userAgent, repoPath: "/" + repository + "/releases/tag/", host: parsed.Host}, nil
}

func (client *ReleaseClient) LatestVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, client.latestURL, nil)
	if err != nil {
		return "", errors.New("create latest release request")
	}
	if client.userAgent != "" {
		request.Header.Set("User-Agent", client.userAgent)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return "", errors.New("failed to request latest GitHub release")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub releases returned HTTP %d", response.StatusCode)
	}
	return client.versionFromURL(response.Request.URL)
}

func (client *ReleaseClient) versionFromURL(value *url.URL) (string, error) {
	if !strings.EqualFold(value.Host, client.host) {
		return "", errors.New("GitHub latest release redirect changed host")
	}
	tag, ok := strings.CutPrefix(value.Path, client.repoPath)
	if !ok || strings.Contains(tag, "/") || strings.TrimSpace(tag) == "" {
		return "", errors.New("GitHub latest release redirect did not contain a version")
	}
	return strings.TrimPrefix(tag, "v"), nil
}

func validRepository(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, current := range part {
			if (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
				(current >= '0' && current <= '9') || current == '-' || current == '_' || current == '.' {
				continue
			}
			return false
		}
	}
	return true
}
