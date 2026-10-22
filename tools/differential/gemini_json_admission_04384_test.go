package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiJSONAdmissionOracleRejectsMatchingCorruptions(t *testing.T) {
	for _, scenario := range []string{"request-json-valid-unicode", "request-json-root-null", "request-json-root-null-rotation", "request-json-root-array", "request-json-root-number", "request-json-root-boolean", "request-json-root-string", "request-json-syntax", "request-json-trailing", "request-json-surrogate", "request-json-utf8", "request-json-overflow", "request-json-depth"} {
		body, _, known := geminiJSONAdmission04384(scenario)
		if !known || body == "" || scenario == "request-json-valid-unicode" && !json.Valid([]byte(body)) {
			t.Fatal("JSON admission fixture lost its original body")
		}
	}
	if !geminiJSONAdmissionNegativeControl04384() {
		t.Fatal("matching admission failure corruptions escaped strict evidence")
	}
}
