package quota

import (
	"math"
	"sort"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
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
		return "openai"
	case quotaProviderGemini:
		return "gemini"
	case quotaProviderAnthropic:
		return "anthropic"
	case quotaProviderCopilot:
		return "copilot"
	case quotaProviderKiro:
		return "kiro"
	case quotaProviderDeepSeek:
		return "deepseek"
	case quotaProviderLocal:
		return "local"
	case quotaProviderAgy:
		return "agy"
	default:
		return "all"
	}
}

func quotaProviderFilterFromString(value string) quotaProviderFilter {
	canonical, ok := normalizeQuotaProviderFilter(value)
	if !ok {
		return quotaProviderAll
	}
	switch canonical {
	case "openai":
		return quotaProviderOpenAI
	case "gemini":
		return quotaProviderGemini
	case "anthropic":
		return quotaProviderAnthropic
	case "copilot":
		return quotaProviderCopilot
	case "kiro":
		return quotaProviderKiro
	case "deepseek":
		return quotaProviderDeepSeek
	case "local":
		return quotaProviderLocal
	case "agy":
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
	auth := report.Auth
	switch filter {
	case quotaProviderOpenAI:
		return strings.EqualFold(auth, "chatgpt")
	case quotaProviderGemini:
		return strings.EqualFold(auth, "gemini")
	case quotaProviderAnthropic:
		return strings.EqualFold(auth, "anthropic")
	case quotaProviderCopilot:
		return strings.EqualFold(auth, "copilot")
	case quotaProviderKiro:
		return strings.EqualFold(auth, "kiro")
	case quotaProviderDeepSeek:
		return strings.EqualFold(auth, "deepseek-key")
	case quotaProviderLocal:
		return strings.EqualFold(auth, "local")
	case quotaProviderAgy:
		return strings.EqualFold(auth, "agy")
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
		return compareQuotaText(left.Usage.PlanType, right.Usage.PlanType)
	default:
		return 0
	}
}

func quotaReportStatusRank(report quotamodel.Report) int {
	if report.Err != nil || strings.EqualFold(report.State, "error") {
		return 2
	}
	if strings.EqualFold(report.State, "ready") {
		return 0
	}
	return 1
}

func quotaReportResetEpoch(report quotamodel.Report) int64 {
	result := int64(math.MaxInt64)
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
	if report.Email != "" {
		return report.Email
	}
	return report.AccountName
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
