package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const runtimeProviderKindRequired = "runtime provider kind is required"

// RunProfile launches model-capable Codex work in an explicit profile home.
// The routing ID is local metadata only; OpenAI credentials and workspace IDs
// continue to come from Codex-owned auth.json in the profile home.
func (runner *Runner) RunProfile(ctx context.Context, codexHome string, args []string) error {
	return runner.RunProfileWithOptions(ctx, codexHome, args, RuntimeLaunchOptions{})
}

func (runner *Runner) RunProfileWithOptions(
	ctx context.Context,
	codexHome string,
	args []string,
	options RuntimeLaunchOptions,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	profileID := profileRoutingID(home)
	return runner.launchHomeWithOptions(
		ctx, home, profileID, proxymodel.Provider{}, nil,
		[]proxymodel.Account{{ID: profileID, Home: home, Enabled: true}}, args, options,
	)
}

func (runner *Runner) RunProviderProfile(ctx context.Context, codexHome string, provider proxymodel.Provider, args []string) error {
	return runner.RunProviderProfileWithOptions(ctx, codexHome, provider, args, RuntimeLaunchOptions{})
}

func (runner *Runner) RunProviderProfileWithOptions(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	args []string,
	options RuntimeLaunchOptions,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if strings.TrimSpace(provider.Kind) == "" {
		return errors.New(runtimeProviderKindRequired)
	}
	profileID := profileRoutingID(provider.Kind + ":" + home)
	return runner.launchHomeWithOptions(
		ctx, home, profileID, provider, nil,
		[]proxymodel.Account{{ID: profileID, Home: home, Enabled: true, Provider: provider}},
		args, options,
	)
}

func (runner *Runner) RunProviderProfiles(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	profiles []proxymodel.ProviderProfile,
	args []string,
) error {
	return runner.RunProviderProfilesWithOptions(
		ctx, codexHome, provider, profiles, args, RuntimeLaunchOptions{},
	)
}

func (runner *Runner) RunProviderProfilesWithOptions(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	profiles []proxymodel.ProviderProfile,
	args []string,
	options RuntimeLaunchOptions,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if strings.TrimSpace(provider.Kind) == "" {
		return errors.New(runtimeProviderKindRequired)
	}
	accounts := make([]proxymodel.Account, 0, len(profiles))
	preferredID := ""
	for _, profile := range profiles {
		currentHome, err := validateRuntimeHome(profile.Home)
		if err != nil {
			return err
		}
		if profile.Provider.Kind != provider.Kind {
			return errors.New("runtime provider pool contains a conflicting provider kind")
		}
		id := profileRoutingID(provider.Kind + ":" + currentHome)
		accounts = append(accounts, proxymodel.Account{
			ID: id, Home: currentHome, Enabled: profile.Enabled,
			RouteOrder: len(accounts) + 1, Provider: profile.Provider,
		})
		if profile.Name == provider.Name && currentHome == home {
			preferredID = id
		}
	}
	if preferredID == "" {
		return errors.New("selected runtime provider profile is missing from the launch pool")
	}
	return runner.launchHomeWithOptions(ctx, home, preferredID, provider, nil, accounts, args, options)
}

func (runner *Runner) RunProviderAPIKeys(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	apiKeys []string,
	args []string,
) error {
	return runner.RunProviderAPIKeysWithOptions(
		ctx, codexHome, provider, apiKeys, args, RuntimeLaunchOptions{},
	)
}

func (runner *Runner) RunProviderAPIKeysWithOptions(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	apiKeys []string,
	args []string,
	options RuntimeLaunchOptions,
) error {
	home, preferredID, accounts, credentials, err := runner.providerAPIKeyPool(codexHome, provider, apiKeys)
	if err != nil {
		return err
	}
	return runner.launchHomeWithOptions(ctx, home, preferredID, provider, credentials, accounts, args, options)
}

func (runner *Runner) providerAPIKeyPool(
	codexHome string,
	provider proxymodel.Provider,
	apiKeys []string,
) (
	string,
	string,
	[]proxymodel.Account,
	[]proxymodel.ProviderCredential,
	error,
) {
	if strings.TrimSpace(provider.Kind) == "" {
		return "", "", nil, nil, errors.New(runtimeProviderKindRequired)
	}
	if len(apiKeys) == 0 {
		return "", "", nil, nil, errors.New("runtime provider API key pool is empty")
	}
	if strings.TrimSpace(codexHome) == "" {
		codexHome = runner.currentHome
	}
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return "", "", nil, nil, err
	}
	accounts := make([]proxymodel.Account, 0, len(apiKeys))
	credentials := make([]proxymodel.ProviderCredential, 0, len(apiKeys))
	seen := make(map[string]bool, len(apiKeys))
	preferredID := ""
	for _, apiKey := range apiKeys {
		if apiKey == "" {
			return "", "", nil, nil, errors.New("runtime provider API key cannot be empty")
		}
		id := providerCredentialRoutingID(provider, apiKey)
		if seen[id] {
			continue
		}
		seen[id] = true
		if preferredID == "" {
			preferredID = id
		}
		accounts = append(accounts, proxymodel.Account{
			ID: id, Home: home, Enabled: true,
			RouteOrder: len(accounts) + 1, Provider: provider,
		})
		credentials = append(credentials, proxymodel.ProviderCredential{ID: id, Secret: apiKey})
	}
	if preferredID == "" {
		return "", "", nil, nil, errors.New("runtime provider API key pool is empty")
	}
	return home, preferredID, accounts, credentials, nil
}

func (runner *Runner) RunProviderAPIKeysAccount(
	ctx context.Context,
	accountID string,
	provider proxymodel.Provider,
	apiKeys []string,
	args []string,
) error {
	return runner.RunProviderAPIKeysAccountWithOptions(
		ctx, accountID, provider, apiKeys, args, RuntimeLaunchOptions{},
	)
}

func (runner *Runner) RunProviderAPIKeysAccountWithOptions(
	ctx context.Context,
	accountID string,
	provider proxymodel.Provider,
	apiKeys []string,
	args []string,
	options RuntimeLaunchOptions,
) (runErr error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("managed account ID is required for provider API-key launch")
	}
	release, err := runner.pinProfiles(ctx, []string{accountID})
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()
	home := runner.accounts.CodexHome(accountID)
	return runner.RunProviderAPIKeysWithOptions(ctx, home, provider, apiKeys, args, options)
}

func providerCredentialRoutingID(provider proxymodel.Provider, secret string) string {
	digest := sha256.Sum256([]byte("provider:" + provider.Kind + "\x00" + provider.APIURL + "\x00" + secret))
	return hex.EncodeToString(digest[:16])
}

func validateRuntimeHome(codexHome string) (string, error) {
	if strings.TrimSpace(codexHome) == "" {
		return "", errors.New("profile CODEX_HOME is required")
	}
	if !filepath.IsAbs(codexHome) {
		return "", errors.New("profile CODEX_HOME must be absolute")
	}
	home := filepath.Clean(codexHome)
	if home == filepath.Dir(home) {
		return "", errors.New("profile CODEX_HOME must not be filesystem root")
	}
	return home, nil
}

func profileRoutingID(home string) string {
	digest := sha256.Sum256([]byte("profile:" + home))
	return hex.EncodeToString(digest[:16])
}
