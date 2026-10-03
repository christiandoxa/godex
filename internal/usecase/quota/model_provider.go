package quota

import (
	"context"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type inspectedProfileQuotaTarget struct {
	target           profilemodel.QuotaTarget
	modelProvider    *profilemodel.ModelProviderSetting
	modelProviderErr error
}

func (status *Status) inspectProfileQuotaTarget(ctx context.Context, target profilemodel.QuotaTarget) inspectedProfileQuotaTarget {
	inspected := inspectedProfileQuotaTarget{target: target}
	if target.Provider != "openai" || status.modelProvider == nil {
		return inspected
	}
	inspected.modelProvider, inspected.modelProviderErr = status.modelProvider.InspectModelProvider(ctx, target.CodexHome)
	if inspected.modelProviderErr != nil {
		inspected.modelProvider = nil
	}
	return inspected
}

func codexModelProviderQuota(setting profilemodel.ModelProviderSetting) *quotamodel.ExternalInfo {
	name := fmt.Sprintf("Custom provider (%s)", setting.ProviderID)
	switch {
	case strings.EqualFold(setting.ProviderID, "prodex-local"):
		name = "Local OpenAI-compatible"
	case strings.EqualFold(setting.ProviderID, "prodex-deepseek"):
		name = "DeepSeek"
	case strings.EqualFold(setting.ProviderID, "prodex-anthropic"):
		name = "Anthropic Claude"
	case strings.EqualFold(setting.ProviderID, "amazon-bedrock"),
		strings.EqualFold(setting.ProviderID, "amazon-bedrock-runtime"),
		strings.EqualFold(setting.ProviderID, "bedrock"):
		name = "Amazon Bedrock"
	}
	return &quotamodel.ExternalInfo{
		Provider: name, Account: setting.ProviderID, Plan: setting.Source,
		Status: "Configured", Main: "quota handled by provider/Codex",
		Details: []quotamodel.ExternalDetail{
			{Label: "Model provider", Value: setting.ProviderID},
			{Label: "Source", Value: setting.Source},
		},
	}
}

func codexModelProviderQuotaJSON(setting profilemodel.ModelProviderSetting) ([]byte, error) {
	return marshalExternalQuotaJSON(*codexModelProviderQuota(setting))
}

func profileProviderFilterMatches(filter string, inspected inspectedProfileQuotaTarget) bool {
	if providerFilterMatches(filter, inspected.target.Provider) {
		return true
	}
	if inspected.modelProvider == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "deepseek":
		return strings.EqualFold(inspected.modelProvider.ProviderID, "prodex-deepseek")
	case "local":
		return strings.EqualFold(inspected.modelProvider.ProviderID, "prodex-local")
	default:
		return false
	}
}
