package main

func geminiControlTypeSuffixes04384() []string {
	return []string{"stream-false", "stream-null", "stream-string", "stream-number", "instructions-null", "instructions-array", "previous-null", "previous-array", "previous-number",
		"model-null", "model-number", "model-bool", "model-array", "model-object", "model-empty", "model-blank", "model-u64", "model-surrogate", "model-utf8", "model-overflow"}
}

func geminiControlTypeCase04384(scenario string) (field, value string, reject, known bool) {
	switch scenario {
	case "request-control-stream-false":
		return "stream", "false", false, true
	case "request-control-stream-null":
		return "stream", "null", true, true
	case "request-control-stream-string":
		return "stream", `"true"`, true, true
	case "request-control-stream-number":
		return "stream", "1", true, true
	case "request-control-instructions-null":
		return "instructions", "null", true, true
	case "request-control-instructions-array":
		return "instructions", "[]", true, true
	case "request-control-previous-null":
		return "previous_response_id", "null", true, true
	case "request-control-previous-array":
		return "previous_response_id", "[]", true, true
	case "request-control-previous-number":
		return "previous_response_id", "7", true, true
	case "request-control-model-null":
		return "model", "null", false, true
	case "request-control-model-number":
		return "model", "7", false, true
	case "request-control-model-bool":
		return "model", "false", false, true
	case "request-control-model-array":
		return "model", "[]", false, true
	case "request-control-model-object":
		return "model", "{}", false, true
	case "request-control-model-empty":
		return "model", `""`, false, true
	case "request-control-model-blank":
		return "model", `" \t "`, false, true
	case "request-control-model-u64":
		return "model", "18446744073709551615", false, true
	case "request-control-model-surrogate":
		return "model", `"\ud800"`, true, true
	case "request-control-model-utf8":
		return "model", "\"\xff\"", true, true
	case "request-control-model-overflow":
		return "model", "1e309", true, true
	}
	return "", "", false, false
}

func geminiControlTypeInput04384(scenario string) []byte {
	field, value, _, _ := geminiControlTypeCase04384(scenario)
	if field == "model" {
		return []byte(`{"input":"same request","model":` + value + `}`)
	}
	return []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","` + field + `":` + value + `}`)
}

func geminiControlTypeExpectation04384(scenario string) string {
	_, _, reject, _ := geminiControlTypeCase04384(scenario)
	if reject {
		return "request-user-id-number"
	}
	return "request-user-id-absent"
}

func geminiControlTypeNegativeControl04384() bool {
	for _, suffix := range geminiControlTypeSuffixes04384() {
		scenario := "request-control-" + suffix
		run := geminiUserIDExpected04384(geminiControlTypeExpectation04384(scenario))
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
