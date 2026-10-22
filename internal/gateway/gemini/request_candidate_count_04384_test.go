package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiCandidateValidationPrecedesToolsAndControls(t *testing.T) {
	const snake = "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	const camel = "invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1"
	const conflict = "invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict"
	const tool = "invalid_tool_declaration: Gemini request field `tools` must be an array"
	for _, fixture := range []struct{ name, fields, message string }{
		{"absent", "", ""},
		{"null", `,"candidate_count":null,"candidateCount":null`, ""},
		{"one", `,"candidate_count":1,"candidateCount":1`, ""},
		{"null-alias", `,"candidate_count":null,"candidateCount":1`, ""},
		{"snake", `,"candidate_count":2`, snake},
		{"camel", `,"candidateCount":2`, camel},
		{"float", `,"candidate_count":1.0`, snake},
		{"string", `,"candidate_count":"1"`, snake},
		{"conflict", `,"candidate_count":1,"candidateCount":2`, conflict},
		{"integer-float", `,"candidate_count":1,"candidateCount":1.0`, conflict},
		{"equal-floats", `,"candidate_count":1.00,"candidateCount":1e0`, snake},
		{"equal-booleans", `,"candidate_count":true,"candidateCount":true`, snake},
		{"equal-arrays", `,"candidate_count":[1.00],"candidateCount":[1e0]`, snake},
		{"equal-objects", `,"candidate_count":{"b":1.00,"a":true},"candidateCount":{"a":true,"b":1e0}`, snake},
		{"equal-strings", `,"candidate_count":"a","candidateCount":"\u0061"`, snake},
		{"different-booleans", `,"candidate_count":true,"candidateCount":false`, conflict},
		{"different-arrays", `,"candidate_count":[1],"candidateCount":[1.0]`, conflict},
		{"tools", `,"candidate_count":2,"tools":false`, snake},
		{"valid-candidate-invalid-tools", `,"candidate_count":1,"tools":false`, tool},
		{"controls", `,"candidateCount":2,"stream":null,"temperature":null`, camel},
		{"input", `,"candidate_count":2,"input":7`, snake},
		{"reasoning", `,"candidate_count":2,"reasoning":"opaque"`, snake},
		{"serde", `,"candidate_count":2,"unused":"\ud800"`, "upstream request failed"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3.1-pro-preview","input":"same request"` + fixture.fields + `}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			if fixture.message == "" {
				if err != nil {
					t.Fatalf("valid candidate count rejected: %v", err)
				}
				return
			}
			status := 400
			if fixture.name == "serde" {
				status = 502
			}
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != status || failure.Message != fixture.message {
				t.Fatalf("candidate validation differs: error=%v want status=%d message=%s", err, status, fixture.message)
			}
		})
	}
}

func TestProdex04384GeminiNativeCandidateAliasesCompareCanonicalSerdeValues(t *testing.T) {
	for _, fields := range []string{
		`"candidate_count":true,"candidateCount":true`,
		`"candidate_count":1.00,"candidateCount":1e0`,
		`"candidate_count":[1.00],"candidateCount":[1e0]`,
		`"candidate_count":{"b":1.00,"a":true},"candidateCount":{"a":true,"b":1e0}`,
		`"candidate_count":"a","candidateCount":"\u0061"`,
	} {
		_, err := translateResponsesRequest([]byte(`{"input":"same request",`+fields+`}`), "gemini-3.1-pro-preview")
		const want = "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
		if err == nil || err.Error() != want {
			t.Fatalf("canonical equal aliases produced wrong native error: fields=%s error=%v", fields, err)
		}
	}
}
