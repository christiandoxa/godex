package runtime

import (
	"bytes"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func TestProdex04371GatewayRejectsActiveOpenAIAPIKeyProfile(t *testing.T) {
	proxy := &gatewayTestProxy{}
	runner := runtimeusecase.NewRunner(nil, nil, func(proxymodel.Config) (runtimeusecase.Proxy, error) {
		return proxy, nil
	})
	profiles := &fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "api-key", CodexHome: t.TempDir(), Provider: "openai", Auth: "api-key",
	}, active: true}
	var output bytes.Buffer
	err := Gateway(t.Context(), runner, profiles, &output, []string{"--listen", "127.0.0.1:0"})
	if err == nil || err.Error() != "gateway provider does not expose an OpenAI-compatible proxy" {
		t.Fatalf("active OpenAI API-key gateway error = %v", err)
	}
	if proxy.started || output.Len() != 0 {
		t.Fatalf("API-key gateway started or reported before rejection: started=%t output=%q", proxy.started, output.String())
	}
}

func TestProdex04371GatewayRejectsOpenAIAPIKeyAccountProfile(t *testing.T) {
	proxy := &gatewayTestProxy{}
	runner := runtimeusecase.NewRunner(nil, nil, func(proxymodel.Config) (runtimeusecase.Proxy, error) {
		return proxy, nil
	})
	profiles := &fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "api-key-account", AccountID: "account", CodexHome: t.TempDir(), Provider: "openai", Auth: "api-key",
	}, active: true}
	var output bytes.Buffer
	err := Gateway(t.Context(), runner, profiles, &output, []string{"--listen", "127.0.0.1:0"})
	if err == nil || err.Error() != "gateway provider does not expose an OpenAI-compatible proxy" {
		t.Fatalf("active OpenAI API-key account gateway error = %v", err)
	}
	if proxy.started || output.Len() != 0 || len(profiles.acquired) != 0 {
		t.Fatalf("API-key account gateway started or acquired before rejection: started=%t output=%q acquired=%v", proxy.started, output.String(), profiles.acquired)
	}
}
