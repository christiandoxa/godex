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

// RunProfile launches model-capable Codex work in an explicit profile home.
// The routing ID is local metadata only; OpenAI credentials and workspace IDs
// continue to come from Codex-owned auth.json in the profile home.
func (runner *Runner) RunProfile(ctx context.Context, codexHome string, args []string) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	profileID := profileRoutingID(home)
	return runner.launchHome(ctx, home, profileID, proxymodel.Provider{}, []proxymodel.Account{{ID: profileID, Home: home, Enabled: true}}, args)
}

func (runner *Runner) RunProviderProfile(ctx context.Context, codexHome string, provider proxymodel.Provider, args []string) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if strings.TrimSpace(provider.Kind) == "" {
		return errors.New("runtime provider kind is required")
	}
	profileID := profileRoutingID(provider.Kind + ":" + home)
	return runner.launchHome(ctx, home, profileID, provider, []proxymodel.Account{{ID: profileID, Home: home, Enabled: true, Provider: provider}}, args)
}

func (runner *Runner) RunProviderProfiles(
	ctx context.Context,
	codexHome string,
	provider proxymodel.Provider,
	profiles []proxymodel.ProviderProfile,
	args []string,
) error {
	home, err := validateRuntimeHome(codexHome)
	if err != nil {
		return err
	}
	if strings.TrimSpace(provider.Kind) == "" {
		return errors.New("runtime provider kind is required")
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
			ID: id, Home: currentHome, Enabled: profile.Enabled, Provider: profile.Provider,
		})
		if profile.Name == provider.Name && currentHome == home {
			preferredID = id
		}
	}
	if preferredID == "" {
		return errors.New("selected runtime provider profile is missing from the launch pool")
	}
	return runner.launchHome(ctx, home, preferredID, provider, accounts, args)
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
