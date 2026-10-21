package chatcompat

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestChatStreamErrorPreservesPresentFieldsAndNumericKinds(t *testing.T) {
	const fallback = "provider_stream_error"
	const generic = "Provider stream returned an embedded error"
	for _, tc := range []struct{ raw, code, message string }{
		{`null`, fallback, generic}, {`[]`, fallback, generic}, {`""`, fallback, ""},
		{`{"type":null,"code":"secondary","message":null,"detail":"secondary"}`, fallback, generic},
		{`{"type":"","message":""}`, "", ""},
		{`{"code":9223372036854775807}`, "9223372036854775807", generic},
		{`{"code":-9223372036854775808}`, "-9223372036854775808", generic},
		{`{"code":9223372036854775808}`, fallback, generic},
		{`{"code":1.0}`, fallback, generic}, {`{"code":-0}`, fallback, generic},
		{`{"code":" E ","detail":"详情 🚀"}`, " E ", "详情 🚀"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			decoder := json.NewDecoder(bytes.NewBufferString(tc.raw))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			code, message := ChatStreamError(value)
			if code != tc.code || message != tc.message {
				t.Errorf("code/message=(%q,%q) want=(%q,%q)", code, message, tc.code, tc.message)
			}
		})
	}
}
