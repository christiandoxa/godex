package ping

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

var allPingEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

const pingModelCacheMaxBytes = 1 << 20

type pingModelCatalog struct {
	id       string
	label    string
	priority uint64
	efforts  []string
}

func normalizePingEffort(model, effort string) (string, error) {
	if strings.TrimSpace(effort) == "" {
		return "", nil
	}
	entries, err := proxymodel.OpenAIProviderCatalog()
	if err != nil {
		return "", err
	}
	selected := strings.TrimSpace(model)
	if selected == "" && len(entries) > 0 {
		selected = entries[0].ID
	}
	entry := proxymodel.ResolveProviderCatalogEntry(entries, selected)
	if entry == nil && len(entries) > 0 {
		entry = &entries[0]
	}
	if entry == nil {
		return "", errors.New("reasoning effort is unsupported for the selected model")
	}
	candidate := strings.ToLower(strings.TrimSpace(effort))
	efforts := entry.SupportedReasoningEfforts
	if len(efforts) == 0 {
		efforts = allPingEfforts
	}
	for _, supported := range efforts {
		if asciiEqualFold(strings.TrimSpace(supported), candidate) {
			return strings.ToLower(strings.TrimSpace(supported)), nil
		}
	}
	return "", errors.New("reasoning effort is unsupported for the selected model")
}

func pingEfforts(model string) []string {
	if dynamic := pingDynamicModel(model); dynamic != nil {
		return append([]string(nil), dynamic.efforts...)
	}
	entries, err := proxymodel.OpenAIProviderCatalog()
	if err != nil {
		return nil
	}
	selected := strings.TrimSpace(model)
	if selected == "" && len(entries) > 0 {
		selected = entries[0].ID
	}
	entry := proxymodel.ResolveProviderCatalogEntry(entries, selected)
	if entry == nil || len(entry.SupportedReasoningEfforts) == 0 {
		return append([]string(nil), allPingEfforts...)
	}
	return append([]string(nil), entry.SupportedReasoningEfforts...)
}

func OpenAIModelChoices() (labels, models []string) { return pingModelChoices() }

func OpenAIEffortChoices(model string) []string { return pingEfforts(model) }

func pingModelChoices() ([]string, []string) {
	entries, err := proxymodel.OpenAIProviderCatalog()
	if err != nil {
		return []string{"provider default"}, []string{""}
	}
	dynamic := pingDynamicModels()
	labels := make([]string, 0, len(entries)+len(dynamic)+1)
	models := make([]string, 0, cap(labels))
	labels = append(labels, "provider default")
	models = append(models, "")
	seen := make(map[string]struct{}, len(entries)+len(dynamic))
	for _, model := range dynamic {
		seen[strings.ToLower(model.id)] = struct{}{}
	}
	for _, entry := range entries {
		if _, exists := seen[strings.ToLower(entry.ID)]; exists {
			continue
		}
		label := strings.TrimSpace(entry.DisplayName)
		if label == "" {
			label = entry.ID
		}
		labels = append(labels, label)
		models = append(models, entry.ID)
	}
	for _, model := range dynamic {
		labels = append(labels, model.label)
		models = append(models, model.id)
	}
	return labels, models
}

func pingDynamicModel(model string) *pingModelCatalog {
	query := strings.TrimSpace(model)
	for _, candidate := range pingDynamicModels() {
		if asciiEqualFold(candidate.id, query) {
			return &candidate
		}
	}
	return nil
}

func pingDynamicModels() []pingModelCatalog {
	path := pingModelCachePath()
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) == 0 || len(content) > pingModelCacheMaxBytes {
		return nil
	}
	var document struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if json.Unmarshal(content, &document) != nil || len(document.Models) == 0 {
		return nil
	}
	models := make([]pingModelCatalog, 0, len(document.Models))
	seen := make(map[string]struct{}, len(document.Models))
	for _, raw := range document.Models {
		id := dynamicString(raw, "slug", "id")
		if id == "" || len(id) > 4096 || pingRetiredModel(id) {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		if supported, present := rawBool(raw, "supported_in_api"); present && !supported {
			continue
		}
		if hidden, present := rawBool(raw, "hidden"); present && hidden {
			continue
		}
		visibility := dynamicString(raw, "visibility")
		if visibility != "" && !asciiEqualFold(visibility, "list") {
			continue
		}
		label := dynamicString(raw, "display_name", "displayName")
		if label == "" {
			label = id
		}
		if len(label) > 65536 {
			return nil
		}
		models = append(models, pingModelCatalog{id: id, label: label, priority: rawPriority(raw), efforts: rawEfforts(raw)})
		seen[key] = struct{}{}
	}
	if len(models) == 0 {
		return nil
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].priority < models[j].priority })
	return models
}

func pingModelCachePath() string {
	home := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(userHome, ".codex")
	}
	return filepath.Join(home, "models_cache.json")
}

func dynamicString(values map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value string
		if json.Unmarshal(values[key], &value) != nil {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func rawBool(values map[string]json.RawMessage, key string) (bool, bool) {
	var value bool
	if json.Unmarshal(values[key], &value) != nil {
		return false, false
	}
	return value, true
}

func rawPriority(values map[string]json.RawMessage) uint64 {
	var value uint64
	if json.Unmarshal(values["priority"], &value) == nil {
		return value
	}
	return math.MaxInt64
}

func rawEfforts(values map[string]json.RawMessage) []string {
	var levels []map[string]json.RawMessage
	if json.Unmarshal(values["supported_reasoning_levels"], &levels) != nil {
		return nil
	}
	result := make([]string, 0, len(levels))
	for _, level := range levels {
		effort := dynamicString(level, "effort")
		if effort == "" || len(effort) > 4096 {
			continue
		}
		duplicate := false
		for _, current := range result {
			if asciiEqualFold(current, effort) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, effort)
		}
	}
	return result
}

func pingRetiredModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "spark", "gpt-5.3-codex-spark", "gpt-5.3-spark":
		return true
	default:
		return false
	}
}

func asciiEqualFold(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftByte, rightByte := left[index], right[index]
		if leftByte >= 'A' && leftByte <= 'Z' {
			leftByte += 'a' - 'A'
		}
		if rightByte >= 'A' && rightByte <= 'Z' {
			rightByte += 'a' - 'A'
		}
		if leftByte != rightByte {
			return false
		}
	}
	return true
}
