package copilot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (source *Source) FetchQuota(ctx context.Context, target profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	host := quotaSnapshotString(target.ProviderConfig.Host)
	login := quotaSnapshotString(target.ProviderConfig.Login)
	if host == "" || login == "" {
		return quotamodel.ExternalInfo{}, errors.New("Copilot quota requires profile host/login metadata")
	}
	config, err := source.readConfig()
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	token, err := source.resolveToken(ctx, config, configUser{Host: host, Login: login})
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	info, err := source.fetchUserInfo(ctx, host, token)
	token = ""
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	return copilotQuotaExternal(info, login), nil
}

func copilotQuotaExternal(info userInfo, fallbackLogin string) quotamodel.ExternalInfo {
	account := normalizedCopilotString(info.Login)
	if account == "" {
		account = strings.TrimSpace(fallbackLogin)
	}
	plan := normalizedCopilotString(info.CopilotPlan)
	access := normalizedCopilotString(info.AccessTypeSKU)
	if plan == "" {
		plan = access
	}
	chatRemaining, chatTotal := copilotQuotaFeature(info, "chat")
	completionRemaining, completionTotal := copilotQuotaFeature(info, "completions")
	ready := copilotQuotaReady(chatRemaining, completionRemaining)
	status := "Ready"
	if !ready {
		status = "Blocked"
	}
	details := make([]quotamodel.ExternalDetail, 0, 3)
	if access != "" && access != plan {
		details = append(details, quotamodel.ExternalDetail{Label: "Access", Value: access})
	}
	if percent, ok := copilotRemainingPercent(chatRemaining, chatTotal, completionRemaining, completionTotal); ok {
		details = append(details, quotamodel.ExternalDetail{Label: "Remaining", Value: fmt.Sprintf("%d%%", percent)})
	}
	reset := ""
	if info.LimitedUserResetDate != nil && strings.TrimSpace(*info.LimitedUserResetDate) != "" {
		reset = "monthly " + strings.TrimSpace(*info.LimitedUserResetDate)
	}
	return quotamodel.ExternalInfo{
		Provider: "GitHub Copilot", Account: account, Plan: plan,
		Status: status, Main: copilotQuotaMain(chatRemaining, chatTotal, completionRemaining, completionTotal),
		Reset: reset, Available: &ready, Details: details,
	}
}

func copilotQuotaFeature(info userInfo, key string) (*int64, *int64) {
	var remaining *int64
	if value, ok := info.LimitedUserQuotas[key]; ok {
		copy := value
		remaining = &copy
	} else if value, ok := info.MonthlyQuotas[key]; ok {
		copy := value
		remaining = &copy
	}
	var total *int64
	if value, ok := info.MonthlyQuotas[key]; ok {
		copy := value
		total = &copy
	}
	return remaining, total
}

func copilotQuotaReady(chatRemaining, completionRemaining *int64) bool {
	return !((chatRemaining != nil && *chatRemaining <= 0) ||
		(completionRemaining != nil && *completionRemaining <= 0))
}

func copilotQuotaMain(chatRemaining, chatTotal, completionRemaining, completionTotal *int64) string {
	parts := make([]string, 0, 2)
	if chatRemaining != nil {
		parts = append(parts, copilotQuotaPart("chat", *chatRemaining, chatTotal))
	}
	if completionRemaining != nil {
		parts = append(parts, copilotQuotaPart("comp", *completionRemaining, completionTotal))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " | ")
}

func copilotQuotaPart(label string, remaining int64, total *int64) string {
	if total == nil {
		return fmt.Sprintf("%s %d", label, remaining)
	}
	return fmt.Sprintf("%s %d/%d", label, remaining, *total)
}

func copilotRemainingPercent(chatRemaining, chatTotal, completionRemaining, completionTotal *int64) (int64, bool) {
	var values []int64
	if chatTotal != nil && *chatTotal > 0 {
		remaining := *chatTotal
		if chatRemaining != nil {
			remaining = *chatRemaining
		}
		values = append(values, roundedPercent(remaining, *chatTotal))
	}
	if completionTotal != nil && *completionTotal > 0 {
		remaining := *completionTotal
		if completionRemaining != nil {
			remaining = *completionRemaining
		}
		values = append(values, roundedPercent(remaining, *completionTotal))
	}
	if len(values) == 0 {
		return 0, false
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum, true
}

func roundedPercent(remaining, total int64) int64 {
	value := float64(remaining) / float64(total) * 100
	if value >= 0 {
		return int64(value + 0.5)
	}
	return int64(value - 0.5)
}

func quotaSnapshotString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizedCopilotString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
