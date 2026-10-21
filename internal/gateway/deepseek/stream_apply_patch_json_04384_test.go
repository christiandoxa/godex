package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

const applyPatchRawArguments04384 = `{"max":18446744073709551615,"large":9007199254740993,"decimal":1e2,"neg":-0,"array":[18446744073709551615],"null":null}`
const applyPatchCanonicalArguments04384 = `{"array":[18446744073709551615],"decimal":100.0,"large":9007199254740993,"max":18446744073709551615,"neg":-0.0,"null":null}`

func TestProdex04384StreamApplyPatchInputPreservesSerdeValues(t *testing.T) {
	for _, fixture := range []struct{ name, arguments, want string }{
		{"ordinary integer", "42", "42"},
		{"unsigned maximum", "18446744073709551615", "18446744073709551615"},
		{"large integer", "9007199254740993", "9007199254740993"},
		{"exponent", "1e2", "100.0"},
		{"negative zero", "-0", "-0.0"},
		{"nested numbers", applyPatchRawArguments04384, applyPatchCanonicalArguments04384},
		{"null", "null", "null"},
		{"boolean", "true", "true"},
		{"explicit string", `"patch text"`, "patch text"},
		{"explicit patch", `{"patch":"*** Begin Patch\r\n*** End Patch"}`, "*** Begin Patch\n*** End Patch"},
		{"content alias", `{"content":"patch text","max":18446744073709551615}`, "patch text"},
		{"edit path", `{"file_path":"sample.txt","content":"new"}`, "*** Begin Patch\n*** Add File: sample.txt\n+new\n*** End Patch"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := deepSeekStreamApplyPatchInput(fixture.arguments); got != fixture.want {
				t.Fatalf("apply_patch input=%s want=%s", got, fixture.want)
			}
		})
	}
}

func TestProdex04384StreamApplyPatchInputRetainsInvalidArgumentFallback(t *testing.T) {
	for _, arguments := range []string{
		"", "{bad", "{} {}", `"\uD800"`, `"\uDC00"`, `{"value":1e309}`,
		strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128),
	} {
		if got := deepSeekStreamApplyPatchInput(arguments); got != arguments {
			t.Errorf("invalid argument fallback changed: got=%q want=%q", got, arguments)
		}
	}
}

func TestProdex04384StreamApplyPatchCompletesWithCanonicalNumericInput(t *testing.T) {
	chunk, err := json.Marshal(map[string]any{
		"id": "chatcmpl-patch", "model": "deepseek-v4-pro", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-patch", "function": map[string]any{
					"name": "apply_patch", "arguments": applyPatchRawArguments04384,
				},
			}}}, "finish_reason": "tool_calls",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := deepSeekChatSSE(io.NopCloser(strings.NewReader("data: " + string(chunk) + "\n\ndata: [DONE]\n\n")))
	defer body.Close()
	output, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(applyPatchCanonicalArguments04384)
	if strings.Count(string(output), `"input":`+string(quoted)) != 2 ||
		strings.Count(string(output), "event: ") != 4 ||
		strings.Contains(string(output), "event: response.failed") {
		t.Fatalf("apply_patch completion lost canonical argument input: %s", output)
	}
}
