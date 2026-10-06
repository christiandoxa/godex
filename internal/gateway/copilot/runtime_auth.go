package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	runtimeIntegrationID = "copilot-developer-cli"
	runtimeAPIVersion    = "2025-04-01"
	runtimeUserAgent     = "copilot/1.0.65 (client/github/cli)"
	runtimeJSONMediaType = "application/json"
	runtimeBodyMaxBytes  = 8 << 20
)

type RuntimeAuth struct {
	apiKey       string
	modelCatalog []map[string]any
}

func (auth RuntimeAuth) modelIDs() []string {
	ids := make([]string, 0, len(auth.modelCatalog))
	for _, model := range auth.modelCatalog {
		if id := runtimeCatalogEntryID(model); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (auth RuntimeAuth) ModelCatalog() []map[string]any {
	result := make([]map[string]any, 0, len(auth.modelCatalog))
	for _, model := range auth.modelCatalog {
		result = append(result, cloneRuntimeCatalogEntry(model))
	}
	return result
}

func (source *Source) NewRuntimeTransport(ctx context.Context, host, login, apiURL string) (*RuntimeTransport, error) {
	return source.NewRuntimeTransportWithClient(ctx, host, login, apiURL, source.client)
}

func (source *Source) NewRuntimeTransportWithClient(
	ctx context.Context,
	host, login, apiURL string,
	client *http.Client,
) (*RuntimeTransport, error) {
	auth, err := source.ResolveRuntimeAuthWithClient(ctx, host, login, client)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = defaultCopilotAPIURL(host)
	}
	return NewRuntimeTransport(apiURL, auth, client)
}

func (source *Source) ResolveRuntimeAuth(ctx context.Context, host, login string) (RuntimeAuth, error) {
	return source.ResolveRuntimeAuthWithClient(ctx, host, login, source.client)
}

func (source *Source) ResolveRuntimeAuthWithClient(ctx context.Context, host, login string, client *http.Client) (RuntimeAuth, error) {
	config, err := source.readConfig()
	if err != nil {
		return RuntimeAuth{}, err
	}
	accessToken, err := source.resolveToken(ctx, config, configUser{Host: host, Login: login})
	if err != nil {
		return RuntimeAuth{}, err
	}
	tokenOrigin, err := copilotUserAPIOrigin(host)
	if err != nil {
		return RuntimeAuth{}, err
	}
	modelsURL := strings.TrimRight(defaultCopilotAPIURL(host), "/") + "/models"
	tokenURL := strings.TrimRight(tokenOrigin, "/") + "/copilot_internal/v2/token"
	if client == nil {
		client = source.client
	}
	auth, err := refreshRuntimeAuth(ctx, client, tokenURL, modelsURL, accessToken)
	accessToken = ""
	return auth, err
}

func refreshRuntimeAuth(ctx context.Context, client *http.Client, tokenURL, modelsURL, accessToken string) (RuntimeAuth, error) {
	direct, directState, directErr := fetchDirectRuntimeAuth(ctx, client, modelsURL, accessToken)
	if directState == directRuntimeReady {
		return direct, nil
	}
	legacy, legacyErr := fetchLegacyRuntimeAuth(ctx, client, tokenURL, accessToken)
	if legacyErr == nil {
		return legacy, nil
	}
	if directErr != nil {
		return RuntimeAuth{}, fmt.Errorf("Copilot runtime auth failed: direct OAuth request failed (%v); legacy token exchange failed (%v)", directErr, legacyErr)
	}
	if directState == directRuntimeUnavailable {
		return RuntimeAuth{apiKey: accessToken}, nil
	}
	return RuntimeAuth{}, legacyErr
}

type directRuntimeState int

const (
	directRuntimeUnavailable directRuntimeState = iota
	directRuntimeReady
)

func fetchDirectRuntimeAuth(ctx context.Context, client *http.Client, modelsURL, accessToken string) (RuntimeAuth, directRuntimeState, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return RuntimeAuth{}, directRuntimeUnavailable, errors.New("create Copilot models request")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", runtimeJSONMediaType)
	request.Header.Set("Content-Type", runtimeJSONMediaType)
	request.Header.Set("Copilot-Integration-Id", runtimeIntegrationID)
	request.Header.Set("x-github-api-version", runtimeAPIVersion)
	request.Header.Set("User-Agent", runtimeUserAgent)
	response, err := client.Do(request)
	if err != nil {
		return RuntimeAuth{}, directRuntimeUnavailable, nil
	}
	body, readErr := readRuntimeBody(response)
	if readErr != nil {
		return RuntimeAuth{}, directRuntimeUnavailable, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return RuntimeAuth{}, directRuntimeUnavailable, fmt.Errorf("Copilot models endpoint rejected direct OAuth with HTTP %d", response.StatusCode)
		}
		return RuntimeAuth{}, directRuntimeUnavailable, nil
	}
	catalog, err := runtimeModelCatalog(body)
	if err != nil {
		return RuntimeAuth{}, directRuntimeUnavailable, nil
	}
	return RuntimeAuth{apiKey: accessToken, modelCatalog: catalog}, directRuntimeReady, nil
}

func fetchLegacyRuntimeAuth(ctx context.Context, client *http.Client, tokenURL, accessToken string) (RuntimeAuth, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return RuntimeAuth{}, errors.New("create Copilot legacy token request")
	}
	request.Header.Set("Authorization", "token "+accessToken)
	request.Header.Set("Accept", runtimeJSONMediaType)
	request.Header.Set("Content-Type", runtimeJSONMediaType)
	request.Header.Set("Editor-Version", "vscode/1.85.1")
	request.Header.Set("Editor-Plugin-Version", "copilot/1.155.0")
	request.Header.Set("User-Agent", "GithubCopilot/1.155.0")
	response, err := client.Do(request)
	if err != nil {
		return RuntimeAuth{}, errors.New("failed to query Copilot legacy token")
	}
	body, err := readRuntimeBody(response)
	if err != nil {
		return RuntimeAuth{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return RuntimeAuth{}, fmt.Errorf("Copilot runtime token exchange failed with HTTP %d", response.StatusCode)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return RuntimeAuth{}, errors.New("failed to parse Copilot runtime token response")
	}
	token, _ := value["token"].(string)
	token = strings.TrimSpace(token)
	if token == "" {
		return RuntimeAuth{}, errors.New("Copilot runtime token response did not contain token")
	}
	catalog, _ := runtimeModelCatalog(body)
	return RuntimeAuth{apiKey: token, modelCatalog: catalog}, nil
}

func readRuntimeBody(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, runtimeBodyMaxBytes+1))
	if err != nil || len(body) > runtimeBodyMaxBytes {
		return nil, errors.New("Copilot runtime auth response exceeded the safe read limit")
	}
	return body, nil
}
