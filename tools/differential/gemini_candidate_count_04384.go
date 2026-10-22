package main

func geminiCandidateCountSuffixes04384() []string {
	return []string{"candidate-null", "candidate-one", "candidate-null-alias", "candidate-snake", "candidate-camel", "candidate-float", "candidate-string",
		"candidate-conflict", "candidate-integer-float", "candidate-equal-floats", "candidate-equal-booleans", "candidate-equal-arrays", "candidate-equal-objects", "candidate-equal-strings",
		"candidate-different-booleans", "candidate-different-arrays", "candidate-tools", "candidate-valid-tools", "candidate-controls", "candidate-input", "candidate-reasoning", "candidate-serde"}
}

func geminiCandidateCountFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	const snake = "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"
	const camel = "invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1"
	const conflict = "invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict"
	fields, message := "", ""
	switch suffix {
	case "candidate-null":
		fields = `"candidate_count":null,"candidateCount":null`
	case "candidate-one":
		fields = `"candidate_count":1,"candidateCount":1`
	case "candidate-null-alias":
		fields = `"candidate_count":null,"candidateCount":1`
	case "candidate-snake":
		fields, message = `"candidate_count":2`, snake
	case "candidate-camel":
		fields, message = `"candidateCount":2`, camel
	case "candidate-float":
		fields, message = `"candidate_count":1.0`, snake
	case "candidate-string":
		fields, message = `"candidate_count":"1"`, snake
	case "candidate-conflict":
		fields, message = `"candidate_count":1,"candidateCount":2`, conflict
	case "candidate-integer-float":
		fields, message = `"candidate_count":1,"candidateCount":1.0`, conflict
	case "candidate-equal-floats":
		fields, message = `"candidate_count":1.00,"candidateCount":1e0`, snake
	case "candidate-equal-booleans":
		fields, message = `"candidate_count":true,"candidateCount":true`, snake
	case "candidate-equal-arrays":
		fields, message = `"candidate_count":[1.00],"candidateCount":[1e0]`, snake
	case "candidate-equal-objects":
		fields, message = `"candidate_count":{"b":1.00,"a":true},"candidateCount":{"a":true,"b":1e0}`, snake
	case "candidate-equal-strings":
		fields, message = `"candidate_count":"a","candidateCount":"\u0061"`, snake
	case "candidate-different-booleans":
		fields, message = `"candidate_count":true,"candidateCount":false`, conflict
	case "candidate-different-arrays":
		fields, message = `"candidate_count":[1],"candidateCount":[1.0]`, conflict
	case "candidate-tools":
		fields, message = `"candidate_count":2,"tools":false`, snake
	case "candidate-valid-tools":
		fields, message = `"candidate_count":1,"tools":false`, "invalid_tool_declaration: Gemini request field `tools` must be an array"
	case "candidate-controls":
		fields, message = `"candidateCount":2,"stream":null,"temperature":null`, camel
	case "candidate-input":
		return requestPrimitiveFixture04384{body: `{"model":"gemini-3.1-pro-preview","input":7,"candidate_count":2}`, reject: true, geminiValidationError: snake}, true
	case "candidate-reasoning":
		fields, message = `"candidate_count":2,"reasoning":"opaque"`, snake
	case "candidate-serde":
		return requestPrimitiveFixture04384{fields: `"candidate_count":2,"unused":"\ud800"`, reject: true}, true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	return requestPrimitiveFixture04384{fields: fields, reject: message != "", geminiValidationError: message}, true
}
