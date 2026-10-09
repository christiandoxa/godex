package runtime

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

// In the tagged Prodex 0.437.0 runtime, supplying a raw external provider
// credential does not bypass the initial compatible-profile selection
// invariant. With an empty profile catalog the gateway must fail closed.
type emptyGatewayProfiles struct{ *superProviderProfiles }

func (emptyGatewayProfiles) ResolveProviderLaunch(context.Context, string, string) (
	profilemodel.LaunchTarget, bool, error,
) {
	return profilemodel.LaunchTarget{}, false, nil
}

func TestProdex04370GatewayRawKeyRequiresAvailableProfileSelection(t *testing.T) {
	profiles := emptyGatewayProfiles{superProviderProfiles: &superProviderProfiles{}}
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	var output bytes.Buffer
	err := Gateway(t.Context(), runner, profiles, &output, []string{
		"--provider", "deepseek",
		"--api-key", "synthetic-only-credential",
		"--base-url", "http://127.0.0.1:12345/v1",
		"--listen", "127.0.0.1:0",
	})
	if err == nil || !strings.Contains(err.Error(), "no active profile selected") {
		t.Fatalf("profileless gateway parity: err=%v output=%q", err, output.String())
	}
	if output.Len() != 0 {
		t.Fatalf("gateway reported listener startup before profile selection: %q", output.String())
	}
}

// A registered compatible profile must still be usable with a legitimate
// synthetic provider key: canonical selection is a precondition, not a
// prohibition on API-key-backed provider gateways.
func TestProdex04370GatewayAPIKeyWithResolvedProfileStillStarts(t *testing.T) {
	proxy := &gatewayTestProxy{}
	var captured proxymodel.Config
	runner := runtimeusecase.NewRunner(nil, nil, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return proxy, nil
	})
	runner.SetProviderCredentialResolver(superCredentialResolver{keys: map[string][]string{
		"deepseek": {"synthetic-only-credential"},
	}})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "managed"))
	profiles := &superProviderProfiles{home: t.TempDir()}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	err := Gateway(ctx, runner, profiles, &output, []string{
		"--provider", "deepseek", "--api-key", "synthetic-only-credential",
		"--base-url", "http://127.0.0.1:12345/v1",
		"--listen", "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("valid resolved provider gateway rejected: %v", err)
	}
	if !proxy.started || !proxy.closed || captured.UpstreamURL != "" {
		t.Fatalf("resolved profile key gateway failed startup/close: started=%t closed=%t config=%#v", proxy.started, proxy.closed, captured)
	}
	if !strings.Contains(output.String(), "Status: listening") {
		t.Fatalf("resolved provider gateway did not produce listening endpoint: %q", output.String())
	}
}
