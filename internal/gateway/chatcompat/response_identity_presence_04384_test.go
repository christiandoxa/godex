package chatcompat

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProdex04384ChatResponseKeepsPresentEmptyIdentityStrings(t *testing.T) {
	for _, tc := range []struct{ raw, wantResponse, wantModel, wantCall string }{
		{`""`, "", "", ""},
		{`"  supplied  "`, "  supplied  ", "  supplied  ", "  supplied  "},
		{`null`, "response-fallback", "model-fallback", "call-fallback"},
		{`false`, "response-fallback", "model-fallback", "call-fallback"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			body := `{"id":` + tc.raw + `,"model":` + tc.raw + `,"choices":[{"message":{"tool_calls":[{"id":` + tc.raw + `,"function":{"name":"lookup","arguments":"{}"}}]}}]}`
			translated, err := ChatResponseWithOptions([]byte(body), time.Unix(1, 0), ResponseOptions{
				DefaultModel: "model-fallback", FallbackResponseID: func() string { return "response-fallback" },
				FallbackCallID: func(int) string { return "call-fallback" },
			})
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal(translated, &response); err != nil {
				t.Fatal(err)
			}
			output, _ := response["output"].([]any)
			if len(output) != 1 {
				t.Fatalf("missing tool output: %s", translated)
			}
			call, _ := output[0].(map[string]any)
			if response["id"] != tc.wantResponse || response["model"] != tc.wantModel || call["call_id"] != tc.wantCall {
				t.Fatalf("present string replaced or non-string borrowed: %s", translated)
			}
		})
	}
}
