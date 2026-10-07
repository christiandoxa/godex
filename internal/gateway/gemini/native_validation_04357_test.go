package gemini

import (
	"strings"
	"testing"
)

func TestProdex04357GeminiValidationReasonsMatchExactTaggedContract(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "candidate aliases conflict",
			body: `{"input":"hello","candidate_count":1,"candidateCount":2}`,
			want: "invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict",
		},
		{
			name: "snake candidate invalid",
			body: `{"input":"hello","candidate_count":2}`,
			want: "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1",
		},
		{
			name: "camel candidate invalid",
			body: `{"input":"hello","candidateCount":1.0}`,
			want: "invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1",
		},
		{
			name: "tools not array",
			body: `{"input":"hello","tools":{"type":"function"}}`,
			want: "invalid_tool_declaration: Gemini request field `tools` must be an array",
		},
		{
			name: "tool not object",
			body: `{"input":"hello","tools":[7]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0]` must be an object",
		},
		{
			name: "wrapped function not object",
			body: `{"input":"hello","tools":[{"type":"function","function":false}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].function` must be an object",
		},
		{
			name: "wrapped function missing name",
			body: `{"input":"hello","tools":[{"type":"function","function":{"description":"x","parameters":{}}}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string",
		},
		{
			name: "flat function missing name",
			body: `{"input":"hello","tools":[{"type":"function","parameters":{}}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].name` must be a non-empty string",
		},
		{
			name: "wrapped function missing parameters",
			body: `{"input":"hello","tools":[{"type":"function","function":{"name":"lookup"}}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required",
		},
		{
			name: "flat function missing parameters",
			body: `{"input":"hello","tools":[{"type":"function","name":"lookup"}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].parameters` is required",
		},
		{
			name: "wrapped parameters wrong type",
			body: `{"input":"hello","tools":[{"type":"function","function":{"name":"lookup","parameters":true}}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object",
		},
		{
			name: "flat description wrong type",
			body: `{"input":"hello","tools":[{"type":"function","name":"lookup","parameters":{},"description":7}]}`,
			want: "invalid_tool_declaration: Gemini request field `tools[0].description` must be a string",
		},
		{
			name: "unsupported response format",
			body: `{"input":"hello","response_format":{"type":"xml"}}`,
			want: "Gemini response_format type `xml` is not supported",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := translateResponsesRequest([]byte(testCase.body), "gemini-3.5-flash")
			if err == nil {
				t.Fatalf("request unexpectedly accepted: %s", testCase.body)
			}
			if err.Error() != testCase.want {
				t.Fatalf("validation error = %q, want %q", err.Error(), testCase.want)
			}
		})
	}
}

func TestProdex04357GeminiStrictValidationKeepsValidFlatAndWrappedFunctions(t *testing.T) {
	for _, body := range []string{
		`{"input":"hello","candidate_count":1,"candidateCount":1,"tools":[{"type":"function","name":"flat","description":"x","parameters":{"type":"object"}}]}`,
		`{"input":"hello","tools":[{"type":"function","function":{"name":"wrapped","description":null,"parameters":{"type":"object"}}}]}`,
	} {
		translated, err := translateResponsesRequest([]byte(body), "gemini-3.5-flash")
		if err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
		if len(translated.body) == 0 || !strings.Contains(string(translated.body), "functionDeclarations") {
			t.Fatalf("valid function declaration was not preserved: %s", translated.body)
		}
	}
}
