package codex

import (
	"context"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func (*CodexProcess) InspectModelProvider(ctx context.Context, home string) (*profilemodel.ModelProviderSetting, error) {
	config, err := readCodexConfig(ctx, home)
	if err != nil {
		return nil, err
	}
	providerID, ok := config["model_provider"].(string)
	if !ok || strings.TrimSpace(providerID) == "" || strings.EqualFold(providerID, "openai") {
		return nil, nil
	}
	return &profilemodel.ModelProviderSetting{ProviderID: providerID, Source: "config.toml"}, nil
}
