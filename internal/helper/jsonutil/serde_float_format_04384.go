package jsonutil

import (
	"strconv"
	"strings"
)

// FormatSerdeDefaultFloat matches the default serde_json 1.0.151 Ryu f64
// presentation used by Prodex 0.438.4: fixed decimal in [1e-5,1e16),
// scientific outside that range, no padded exponent, and a `.0` suffix
// for otherwise integer-looking floating-point values. The caller has
// already obtained the f64 using ParseSerdeDefaultFloat.
func FormatSerdeDefaultFloat(value float64) string {
	magnitude := value
	if magnitude < 0 {
		magnitude = -magnitude
	}
	if magnitude == 0 || magnitude >= 1e-5 && magnitude < 1e16 {
		formatted := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.Contains(formatted, ".") {
			formatted += ".0"
		}
		return formatted
	}
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	if index := strings.LastIndexByte(formatted, 'e'); index >= 0 && index+2 < len(formatted) {
		prefix, exponent := formatted[:index+2], formatted[index+2:]
		exponent = strings.TrimLeft(exponent, "0")
		if exponent == "" {
			exponent = "0"
		}
		formatted = prefix + exponent
	}
	return formatted
}
