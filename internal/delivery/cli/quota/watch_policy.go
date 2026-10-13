package quota

import (
	"math"
	"sort"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	quotaProviderLabelAll       = "all"
	quotaProviderLabelOpenAI    = "openai"
	quotaProviderLabelGemini    = "gemini"
	quotaProviderLabelAnthropic = "anthropic"
	quotaProviderLabelCopilot   = "copilot"
	quotaProviderLabelKiro      = "kiro"
	quotaProviderLabelDeepSeek  = "deepseek"
	quotaProviderLabelLocal     = "local"
	quotaProviderLabelAgy       = "agy"
)

type quotaReportSort uint8

const (
	quotaSortCurrent quotaReportSort = iota
	quotaSortRemaining
	quotaSortProfile
	quotaSortAuth
	quotaSortAccount
	quotaSortPlan
)

func (sortMode quotaReportSort) next() quotaReportSort {
	return (sortMode + 1) % 6
}

func (sortMode quotaReportSort) label() string {
	switch sortMode {
	case quotaSortCurrent:
		return "current"
	case quotaSortRemaining:
		return "remaining"
	case quotaSortProfile:
		return "profile"
	case quotaSortAuth:
		return "auth"
	case quotaSortAccount:
		return "account"
	case quotaSortPlan:
		return "plan"
	default:
		return "current"
	}
}

type quotaProviderFilter uint8

const (
	quotaProviderAll quotaProviderFilter = iota
	quotaProviderOpenAI
	quotaProviderGemini
	quotaProviderAnthropic
	quotaProviderCopilot
	quotaProviderKiro
	quotaProviderDeepSeek
	quotaProviderLocal
	quotaProviderAgy
)

func (filter quotaProviderFilter) next() quotaProviderFilter {
	return (filter + 1) % 9
}

func (filter quotaProviderFilter) label() string {
	switch filter {
	case quotaProviderOpenAI:
		return quotaProviderLabelOpenAI
	case quotaProviderGemini:
		return quotaProviderLabelGemini
	case quotaProviderAnthropic:
		return quotaProviderLabelAnthropic
	case quotaProviderCopilot:
		return quotaProviderLabelCopilot
	case quotaProviderKiro:
		return quotaProviderLabelKiro
	case quotaProviderDeepSeek:
		return quotaProviderLabelDeepSeek
	case quotaProviderLocal:
		return quotaProviderLabelLocal
	case quotaProviderAgy:
		return quotaProviderLabelAgy
	default:
		return quotaProviderLabelAll
	}
}

func quotaProviderFilterFromString(value string) quotaProviderFilter {
	canonical, ok := normalizeQuotaProviderFilter(value)
	if !ok {
		return quotaProviderAll
	}
	switch canonical {
	case quotaProviderLabelOpenAI:
		return quotaProviderOpenAI
	case quotaProviderLabelGemini:
		return quotaProviderGemini
	case quotaProviderLabelAnthropic:
		return quotaProviderAnthropic
	case quotaProviderLabelCopilot:
		return quotaProviderCopilot
	case quotaProviderLabelKiro:
		return quotaProviderKiro
	case quotaProviderLabelDeepSeek:
		return quotaProviderDeepSeek
	case quotaProviderLabelLocal:
		return quotaProviderLocal
	case quotaProviderLabelAgy:
		return quotaProviderAgy
	default:
		return quotaProviderAll
	}
}

func (filter quotaProviderFilter) matches(report quotamodel.Report) bool {
	if filter == quotaProviderAll {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(report.Provider), filter.label()) {
		return true
	}
	if info := report.External; info != nil {
		switch filter {
		case quotaProviderDeepSeek:
			if strings.EqualFold(info.Provider, "DeepSeek") || strings.EqualFold(info.Account, "prodex-deepseek") {
				return true
			}
		case quotaProviderLocal:
			if strings.EqualFold(info.Provider, "Local OpenAI-compatible") || (strings.EqualFold(info.Account, "godex-local") || strings.EqualFold(info.Account, "prodex-local")) {
				return true
			}
		}
	}
	auth := report.Auth
	switch filter {
	case quotaProviderOpenAI:
		return strings.EqualFold(auth, "chatgpt")
	case quotaProviderGemini:
		return strings.EqualFold(auth, quotaProviderLabelGemini)
	case quotaProviderAnthropic:
		return strings.EqualFold(auth, quotaProviderLabelAnthropic)
	case quotaProviderCopilot:
		return strings.EqualFold(auth, quotaProviderLabelCopilot)
	case quotaProviderKiro:
		return strings.EqualFold(auth, quotaProviderLabelKiro)
	case quotaProviderDeepSeek:
		return strings.EqualFold(auth, "deepseek-key")
	case quotaProviderLocal:
		return strings.EqualFold(auth, quotaProviderLabelLocal)
	case quotaProviderAgy:
		return strings.EqualFold(auth, quotaProviderLabelAgy)
	default:
		return false
	}
}

func quotaSortedReports(reports []quotamodel.Report, filter quotaProviderFilter, sortMode quotaReportSort) []quotamodel.Report {
	result := make([]quotamodel.Report, 0, len(reports))
	for _, report := range reports {
		if filter.matches(report) {
			result = append(result, report)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		comparison := compareQuotaReports(result[i], result[j], sortMode)
		if comparison == 0 {
			return quotaReportName(result[i]) < quotaReportName(result[j])
		}
		return comparison < 0
	})
	return result
}

func compareQuotaReports(left, right quotamodel.Report, sortMode quotaReportSort) int {
	switch sortMode {
	case quotaSortCurrent:
		if left.Active != right.Active {
			if left.Active {
				return -1
			}
			return 1
		}
		return compareInt(quotaReportStatusRank(left), quotaReportStatusRank(right))
	case quotaSortRemaining:
		if rank := compareInt(quotaReportStatusRank(left), quotaReportStatusRank(right)); rank != 0 {
			return rank
		}
		return compareInt64(quotaReportResetEpoch(left), quotaReportResetEpoch(right))
	case quotaSortProfile:
		return compareQuotaText(quotaReportName(left), quotaReportName(right))
	case quotaSortAuth:
		return compareQuotaText(left.Auth, right.Auth)
	case quotaSortAccount:
		return compareQuotaText(quotaReportAccount(left), quotaReportAccount(right))
	case quotaSortPlan:
		return compareQuotaText(quotaReportPlan(left), quotaReportPlan(right))
	default:
		return 0
	}
}

func quotaReportStatusRank(report quotamodel.Report) int {
	if report.Err != nil || strings.EqualFold(report.State, "error") {
		return 2
	}
	if report.External != nil {
		if report.External.Available != nil && *report.External.Available {
			return 0
		}
		return 1
	}
	if strings.EqualFold(report.Provider, "openai") || report.Provider == "" {
		// Tagged Prodex sorts OpenAI reports by quota-capacity candidates,
		// not by the separately rendered, possibly stale status label.
		if quotaOpenAIHasReadyLimit04379(report.Usage) {
			return 0
		}
		return 1
	}
	if strings.EqualFold(report.State, "ready") {
		return 0
	}
	return 1
}

func quotaOpenAIHasReadyLimit04379(usage quotamodel.Usage) bool {
	if usage.Allowed != nil && !*usage.Allowed ||
		usage.LimitReached != nil && *usage.LimitReached {
		return false
	}
	known := false
	for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
		if window == nil || window.UsedPercent == nil {
			continue
		}
		known = true
		if *window.UsedPercent >= 100 {
			return false
		}
	}
	return known
}

func quotaReportResetEpoch(report quotamodel.Report) int64 {
	result := int64(math.MaxInt64)
	if report.External != nil {
		// In tagged Prodex generic external snapshots have no reset sort
		// key. Copilot/Gemini own separate provider-specific reset data.
		if (strings.EqualFold(report.Provider, "copilot") ||
			strings.EqualFold(report.Provider, "gemini")) &&
			report.External.ResetAt != nil {
			return *report.External.ResetAt
		}
		return result
	}
	if report.Provider != "" && !strings.EqualFold(report.Provider, "openai") &&
		!strings.EqualFold(report.Provider, "gemini") &&
		!strings.EqualFold(report.Provider, "copilot") {
		return result
	}
	for _, window := range []*quotamodel.Window{report.Usage.Primary, report.Usage.Secondary} {
		if window != nil && window.ResetAt != nil && *window.ResetAt < result {
			result = *window.ResetAt
		}
	}
	return result
}

func quotaReportName(report quotamodel.Report) string {
	if report.ProfileName != "" {
		return report.ProfileName
	}
	return report.AccountName
}

func quotaReportAccount(report quotamodel.Report) string {
	if report.External != nil && strings.TrimSpace(report.External.Account) != "" {
		return report.External.Account
	}
	if report.Email != "" {
		return report.Email
	}
	return report.AccountName
}

func quotaReportPlan(report quotamodel.Report) string {
	if report.External != nil {
		return report.External.Plan
	}
	return report.Usage.PlanType
}

func compareQuotaText(left, right string) int {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	return strings.Compare(left, right)
}

func compareInt(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
