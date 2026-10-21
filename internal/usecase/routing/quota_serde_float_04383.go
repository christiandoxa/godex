package routing

import "github.com/christiandoxa/godex/internal/helper/jsonutil"

// Retain the tagged routing helper while sharing the checked serde_json
// default f64 parser with Kiro ACP envelope validation.
func quotaSerdeFloat04383(value string) (float64, bool) {
	return jsonutil.ParseSerdeDefaultFloat(value)
}
