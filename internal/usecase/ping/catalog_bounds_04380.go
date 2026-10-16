package ping

import (
	"encoding/json"
	"strings"
)

const (
	pingCatalogMaxModels04380    = 1024
	pingCatalogMaxIDBytes04380   = 4096
	pingCatalogMaxTextBytes04380 = 65536
)

// Prodex 0.438.0 rich_catalog.mojo validates every input model, alias and
// effort before the planner filters, sorts and de-duplicates choices. A
// malformed oversized entry must invalidate the *whole dynamic cache*, not
// silently allow an incorrect partial set of models next to bundled choices.
func pingCatalogInputValid04380(models []map[string]json.RawMessage) bool {
	if len(models) > pingCatalogMaxModels04380 {
		return false
	}
	for _, entry := range models {
		if len(catalogFirstString04380(entry, "slug", "id")) > pingCatalogMaxIDBytes04380 ||
			len(catalogFirstString04380(entry, "display_name", "displayName")) > pingCatalogMaxTextBytes04380 ||
			len(catalogFirstString04380(entry, "default_reasoning_level", "default_reasoning_effort")) > pingCatalogMaxTextBytes04380 {
			return false
		}
		var aliases []json.RawMessage
		if json.Unmarshal(entry["aliases"], &aliases) == nil {
			for _, rawAlias := range aliases {
				var alias *string
				if json.Unmarshal(rawAlias, &alias) == nil && alias != nil &&
					len(strings.TrimSpace(*alias)) > pingCatalogMaxIDBytes04380 {
					return false
				}
			}
		}
		var levels []json.RawMessage
		if json.Unmarshal(entry["supported_reasoning_levels"], &levels) == nil {
			for _, rawLevel := range levels {
				var level map[string]json.RawMessage
				if json.Unmarshal(rawLevel, &level) == nil &&
					len(catalogFirstString04380(level, "effort")) > pingCatalogMaxTextBytes04380 {
					return false
				}
			}
		}
	}
	return true
}

// Same first-well-typed-key rule as tagged catalog_entry_model_id: an empty
// `slug` still wins over a later `id`; the model is subsequently filtered.
func catalogFirstString04380(values map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value *string
		if json.Unmarshal(values[key], &value) == nil && value != nil {
			return strings.TrimSpace(*value)
		}
	}
	return ""
}
