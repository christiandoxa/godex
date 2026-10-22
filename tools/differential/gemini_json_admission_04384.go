package main

import (
	"errors"
	"net/http"
	"strings"
)

func geminiJSONAdmission04384(scenario string) (body string, reject, known bool) {
	const request = `{"model":"gemini-3.1-pro-preview","input":"same request"`
	switch scenario {
	case "request-json-valid-unicode":
		return request + `,"unused":"\ud83d\ude00"}`, false, true
	case "request-json-root-null", "request-json-root-null-rotation":
		return "null", true, true
	case "request-json-root-array":
		return "[]", true, true
	case "request-json-root-number":
		return "7", true, true
	case "request-json-root-boolean":
		return "true", true, true
	case "request-json-root-string":
		return `"opaque"`, true, true
	case "request-json-syntax":
		return `{"input":`, true, true
	case "request-json-trailing":
		return request + `} {}`, true, true
	case "request-json-surrogate":
		return request + `,"unused":"\ud800"}`, true, true
	case "request-json-utf8":
		return request + `,"unused":"` + string([]byte{0xff}) + `"}`, true, true
	case "request-json-overflow":
		return request + `,"unused":1e309}`, true, true
	case "request-json-depth":
		return request + `,"unused":` + strings.Repeat("[", 140) + "0" + strings.Repeat("]", 140) + "}", true, true
	}
	return "", false, false
}

func geminiJSONAdmissionExpectation04384(scenario string) string {
	_, reject, _ := geminiJSONAdmission04384(scenario)
	if reject {
		return "request-user-id-number"
	}
	return "request-user-id-absent"
}

func geminiJSONAdmissionExpected04384(scenario string) geminiCandidateResult {
	result := geminiUserIDExpected04384(geminiJSONAdmissionExpectation04384(scenario))
	if strings.HasPrefix(scenario, "request-json-root-") {
		result.Client = exchange{Status: 500, Headers: make(http.Header)}
		result.Stdout = "\n"
	}
	return result
}

func geminiJSONAdmissionInvariant04384(scenario string, result geminiCandidateResult) error {
	if strings.HasPrefix(scenario, "request-json-root-") {
		if result.Client.Status != 500 || result.Client.Body != "" || result.Client.Headers.Get("Content-Type") != "" ||
			result.ExitCode != 2 || result.Stdout != "\n" || len(result.Upstream) != 0 || len(result.StateIntegrity) != 0 || result.ClientError != "" {
			return errors.New("Gemini valid nonobject root did not produce the exact empty terminal 500")
		}
		return nil
	}
	if err := geminiUserIDInvariant04384(geminiJSONAdmissionExpectation04384(scenario), result); err != nil {
		return errors.New("Gemini JSON admission lost exact prepared failure or accepted original request")
	}
	return nil
}

func geminiJSONAdmissionNegativeControl04384() bool {
	for _, scenario := range []string{"request-json-valid-unicode", "request-json-root-null", "request-json-root-null-rotation", "request-json-root-array", "request-json-root-number", "request-json-root-boolean", "request-json-root-string", "request-json-syntax", "request-json-trailing", "request-json-surrogate", "request-json-utf8", "request-json-overflow", "request-json-depth"} {
		run := geminiJSONAdmissionExpected04384(scenario)
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "application/json") },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.ClientError = "missing exchange" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		} {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			changed.Upstream = append([]geminiCapturedRequest(nil), run.Upstream...)
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
	}
	return true
}
