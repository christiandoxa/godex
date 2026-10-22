package deepseek

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekToolSchemasRetainSerdeNumbers(t *testing.T) {
	for _, fixture := range []struct{ name, input, want string }{
		{"integer", "7", "7"},
		{"above_exact_float", "9007199254740993", "9007199254740993"},
		{"unsigned", "18446744073709551615", "18446744073709551615"},
		{"signed", "-9223372036854775808", "-9223372036854775808"},
		{"float_control", "1e0,-0,0.123456789012345678901234567890", "1.0,-0.0,0.12345678901234568"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := `{"input":"same request","tools":[{"type":"function","name":"lookup","parameters":{"type":"integer","enum":[` + fixture.input + `]}}]}`
			body, err := ResponsesRequest([]byte(input), RequestOptions{})
			want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stream":false,"tools":[{"function":{"name":"lookup","parameters":{"enum":[` + fixture.want + `],"type":"integer"}},"type":"function"}]}`
			if err != nil || string(body) != want {
				t.Fatalf("tool schema changed locked Serde number bytes: error=%v body=%s", err, body)
			}
		})
	}
}

func TestProdex04384DeepSeekStrictSchemasRetainNumbers(t *testing.T) {
	body, err := ResponsesRequest([]byte(`{"input":"same request","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"value":{"type":"integer","enum":[9007199254740993]}}}}]}`), RequestOptions{StrictTools: true})
	want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stream":false,"tools":[{"function":{"name":"lookup","parameters":{"additionalProperties":false,"properties":{"value":{"enum":[9007199254740993],"type":"integer"}},"required":["value"],"type":"object"},"strict":true},"type":"function"}]}`
	if err != nil || string(body) != want {
		t.Fatalf("strict normalization changed exact enum: error=%v body=%s", err, body)
	}
}

func TestProdex04384DeepSeekDistinctIntegerSchemasDoNotCollapse(t *testing.T) {
	input := `{"input":"same request","tools":[{"type":"function","name":"lookup","parameters":{"enum":[9007199254740992]}},{"type":"function","name":"lookup","parameters":{"enum":[9007199254740993]}}]}`
	if _, err := ResponsesRequest([]byte(input), RequestOptions{}); err == nil || !strings.Contains(err.Error(), "duplicated after translation") {
		t.Fatalf("distinct integer schemas collapsed: %v", err)
	}
}

func TestProdex04384DeepSeekRequestMetadataRetainsNumbersThroughResponse(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"metadata", `"metadata":{"probe":{"value":18446744073709551615}}`, `{"metadata":{"probe":{"value":18446744073709551615}}}`},
		{"client", `"client_metadata":{"value":9007199254740993}`, `{"metadata":{"client_metadata":{"value":9007199254740993}}}`},
		{"nested", `"metadata":{"probe":[{"value":-9223372036854775808}]}`, `{"metadata":{"probe":[{"value":-9223372036854775808}]}}`},
		{"float_control", `"metadata":{"probe":{"value":1.5}}`, `{"metadata":{"probe":{"value":1.5}}}`},
		{"exponent", `"metadata":{"probe":{"value":1e0}}`, `{"metadata":{"probe":{"value":1.0}}}`},
		{"negative_zero", `"metadata":{"probe":{"value":-0}}`, `{"metadata":{"probe":{"value":-0.0}}}`},
		{"fraction", `"metadata":{"probe":{"value":0.123456789012345678901234567890}}`, `{"metadata":{"probe":{"value":0.12345678901234568}}}`},
		{"client_negative_zero", `"client_metadata":{"value":-0}`, `{"metadata":{"client_metadata":{"value":-0.0}}}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			request, err := TranslateResponsesRequest([]byte(`{"input":"same request",`+fixture.fields+`}`), RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			response, err := translatedDeepSeekResponseWithContentType(&http.Response{StatusCode: 200}, []byte(`{}`), request.ResponseMetadata, "application/json")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != fixture.want {
				t.Fatalf("metadata number changed through response merge: error=%v body=%s", err, body)
			}
		})
	}
}
