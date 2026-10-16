package main

import "testing"

func TestProdex04380GeminiInvalidToolDifferentialRequiresIndexedError(t *testing.T) {
	for _, scenario := range []string{"invalid-wrapped-tool-parameters", "invalid-flat-tool-parameters"} {
		if !geminiInvalidToolScenario04380(scenario) {
			t.Fatalf("unknown fixture %q", scenario)
		}
		field, detail := "tools[0].function.parameters", "must be an object"
		if scenario == "invalid-flat-tool-parameters" {
			detail = "is required"
		}
		canonical := geminiCandidateResult{Name: "prodex", ExitCode: 2,
			Client: exchange{Status: 400, Body: `{"error":{"message":"invalid_tool_declaration: Gemini request field ` + "`" + field + "`" + ` ` + detail + `","code":"invalid_request_error"}}`},
		}
		if err := geminiInvalidToolInvariant04380(scenario, canonical); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []geminiCandidateResult{
			{ExitCode: 1, Client: canonical.Client},
			{ExitCode: 2, Client: exchange{Status: 200, Body: canonical.Client.Body}},
			{ExitCode: 2, Client: exchange{Status: 400, Body: `{"error":{"message":"missing indexed field path"}}`}},
			{ExitCode: 2, Client: canonical.Client, Upstream: []geminiCapturedRequest{{Path: "/v1beta/openai/chat/completions"}}},
		} {
			if err := geminiInvalidToolInvariant04380(scenario, bad); err == nil {
				t.Fatalf("invalid diagnostic/status/exit or provider-leak accepted: %+v", bad)
			}
		}
		if !geminiInvalidToolWireEqual04380(canonical.Client.Body, canonical.Client.Body) {
			t.Fatal("matching canonical rejection rejected")
		}
		if geminiInvalidToolWireEqual04380(canonical.Client.Body, `{"error":{"message":"different"}}`) {
			t.Fatal("mutated structured rejection evaded comparison")
		}
	}
	if geminiInvalidToolScenario04380("multi-turn-tool") {
		t.Fatal("valid fixture incorrectly rejected")
	}
}
