package copilot

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

type Source struct {
	client  *http.Client
	getenv  func(string) string
	homeDir func() (string, error)
	run     commandRunner
}

func NewSource(client *http.Client) *Source {
	if client == nil {
		client = defaultHTTPClient()
	}
	return &Source{client: client, getenv: os.Getenv, homeDir: os.UserHomeDir, run: runCredentialCommand}
}

func (source *Source) Load(ctx context.Context) (profilemodel.BuiltinCredential, error) {
	config, err := source.readConfig()
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	users := candidateUsers(config)
	if len(users) == 0 {
		return profilemodel.BuiltinCredential{}, errors.New("no logged-in Copilot user found in config.json")
	}
	selected, token, err := source.selectUserToken(ctx, config, users)
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	info, err := source.fetchUserInfo(ctx, selected.Host, token)
	token = ""
	if err != nil {
		return profilemodel.BuiltinCredential{}, err
	}
	return copilotCredential(selected, info), nil
}

func (source *Source) selectUserToken(ctx context.Context, config configFile, users []configUser) (configUser, string, error) {
	for _, user := range users {
		token, err := source.resolveToken(ctx, config, user)
		if err == nil && strings.TrimSpace(token) != "" {
			return user, token, nil
		}
	}
	return configUser{}, "", errors.New("failed to resolve a stored Copilot token from config or keychain")
}

func copilotCredential(selected configUser, info userInfo) profilemodel.BuiltinCredential {
	login := strings.TrimSpace(selected.Login)
	if info.Login != nil && strings.TrimSpace(*info.Login) != "" {
		login = strings.TrimSpace(*info.Login)
	}
	apiURL := ""
	if info.Endpoints != nil && info.Endpoints.API != nil {
		apiURL = strings.TrimSpace(*info.Endpoints.API)
	}
	if apiURL == "" {
		apiURL = defaultCopilotAPIURL(selected.Host)
	}
	provider := profilemodel.ProviderSnapshot{
		Kind: "copilot", Host: stringPointer(strings.TrimSpace(selected.Host)), Login: stringPointer(login),
		APIURL: stringPointer(apiURL), AccessTypeSKU: normalizedPointer(info.AccessTypeSKU), CopilotPlan: normalizedPointer(info.CopilotPlan),
	}
	return profilemodel.BuiltinCredential{Provider: provider, Email: strings.TrimSpace(selected.Login)}
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}

func normalizedPointer(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return stringPointer(strings.TrimSpace(*value))
}
