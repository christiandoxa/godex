package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (virtual *Virtual) collectAgy(ctx context.Context) []quotamodel.VirtualResult {
	info, err := virtual.fetchAgy(ctx)
	if err != nil {
		return []quotamodel.VirtualResult{{Name: virtualProviderAgy, Provider: virtualProviderAgy, Auth: virtualProviderAgy, Err: err}}
	}
	name := virtualProviderAgy
	if strings.TrimSpace(info.Account) != "" {
		name += ":" + strings.TrimSpace(info.Account)
	}
	return []quotamodel.VirtualResult{{Name: name, Provider: virtualProviderAgy, Auth: virtualProviderAgy, External: &info}}
}

func (virtual *Virtual) fetchAgy(ctx context.Context) (quotamodel.ExternalInfo, error) {
	result, err := virtual.run(ctx, "agy", []string{"auth", "quota", "--format=json", "--detail", "--all-accounts"})
	if err != nil {
		return quotamodel.ExternalInfo{}, err
	}
	if result.exitCode != 0 {
		return quotamodel.ExternalInfo{}, errors.New("agy quota command failed")
	}
	if len(result.stdout) == 0 || len(result.stdout) > virtualBodyMaxBytes {
		return quotamodel.ExternalInfo{}, errors.New("agy quota command returned invalid output")
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(result.stdout)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return quotamodel.ExternalInfo{}, errors.New("failed to parse agy quota JSON output")
	}
	return parseAgyQuota(value)
}

func parseAgyQuota(value any) (quotamodel.ExternalInfo, error) {
	accounts := agyAccounts(value)
	if len(accounts) == 0 {
		return quotamodel.ExternalInfo{}, errors.New("no account found in agy output")
	}
	data := accounts[0]
	account := firstAgyString(data, "account", "email")
	plan := firstAgyString(data, "plan")
	status := firstAgyString(data, "status")
	if status == "" {
		status = "Ready"
	}
	available := true
	return quotamodel.ExternalInfo{
		Provider: "Anti-Gravity", Account: account, Plan: plan, Status: status,
		Main: agyMain(data), Available: &available, Details: agyDetails(data),
	}, nil
}

func agyAccounts(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, raw := range typed {
			if account, ok := raw.(map[string]any); ok {
				result = append(result, account)
			}
		}
		return result
	case map[string]any:
		return []map[string]any{typed}
	default:
		return nil
	}
}

func agyMain(data map[string]any) string {
	if credits, ok := numberAsFloat(data["credits"]); ok {
		return fmt.Sprintf("%.2f credits available", credits)
	}
	if usage, ok := numberAsFloat(data["usage"]); ok {
		return fmt.Sprintf("%.2f usage", usage)
	}
	return "quota information available"
}

func agyDetails(data map[string]any) []quotamodel.ExternalDetail {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var details []quotamodel.ExternalDetail
	for _, key := range keys {
		value := data[key]
		if agyIdentityField(key) {
			continue
		}
		if key == "details" || key == "models" || key == "quotas" {
			details = append(details, flattenAgyDetail(key, value)...)
			continue
		}
		details = append(details, quotamodel.ExternalDetail{Label: key, Value: quotaScalar(value)})
	}
	return details
}

func flattenAgyDetail(label string, value any) []quotamodel.ExternalDetail {
	var result []quotamodel.ExternalDetail
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result = append(result, quotamodel.ExternalDetail{Label: label + ":" + key, Value: quotaScalar(typed[key])})
		}
	case []any:
		for index, item := range typed {
			result = append(result, quotamodel.ExternalDetail{Label: fmt.Sprintf("%s[%d]", label, index), Value: quotaScalar(item)})
		}
	}
	return result
}

func firstAgyString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	return ""
}

func agyIdentityField(key string) bool {
	switch key {
	case "account", "email", "plan", "status", "credits", "usage":
		return true
	default:
		return false
	}
}

func numberAsFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func quotaScalar(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, err := json.Marshal(value)
	if err != nil {
		return "-"
	}
	return string(content)
}
