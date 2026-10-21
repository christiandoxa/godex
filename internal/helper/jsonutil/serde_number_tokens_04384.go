package jsonutil

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// CheckSerdeNumberTokens rejects numeric literals that serde_json 1.0.151
// rejects before any owning typed-struct validation. It observes nested
// numbers through Decoder.Token rather than lossy Go float64 conversion.
// The caller checks strict UTF-8/surrogates/container depth separately.
func CheckSerdeNumberTokens(payload []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
		if number, ok := token.(json.Number); ok && !validSerdeJSONNumber(string(number)) {
			return false
		}
	}
}

func validSerdeJSONNumber(number string) bool {
	if !strings.ContainsAny(number, ".eE") && number != "-0" {
		if strings.HasPrefix(number, "-") {
			if _, err := strconv.ParseInt(number, 10, 64); err == nil {
				return true
			}
		} else if _, err := strconv.ParseUint(number, 10, 64); err == nil {
			return true
		}
	}
	_, ok := ParseSerdeDefaultFloat(number)
	return ok
}
