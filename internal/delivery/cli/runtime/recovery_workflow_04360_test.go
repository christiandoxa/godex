package runtime

import (
	"encoding/json"
	"testing"
)

func TestProdex04360StructuredWorkflowRecoveryClassesMatchTaggedSource(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"legacy usage", `{"type":"error","error":{"code":"usage_limit_reached"}}`, "usage_limit"},
		{"native usage", `{"type":"error","error":{"codex_error_info":{"usage_limit_exceeded":{}}}}`, "usage_limit"},
		{"native rate", `{"type":"event_msg","payload":{"type":"error","codex_error_info":"rate_limit_exceeded"}}`, "rate_limit"},
		{"camel rate", `{"type":"error","error":{"codexErrorInfo":"rateLimitExceeded"}}`, "rate_limit"},
		{"server overload", `{"type":"error","error":{"codex_error_info":{"serverOverloaded":{}}}}`, "overload"},
		{"auth failure", `{"type":"turn.failed","error":{"codex_error_info":"unauthorized"}}`, "auth"},
		{"transport", `{"type":"error","error":{"codex_error_info":"responseStreamDisconnected"}}`, "transport"},
		{"failed turn completion", `{"method":"turn/completed","params":{"turn":{"status":"failed","error":{"codex_error_info":"usageLimitExceeded"}}}}`, "usage_limit"},
		{"safe completed turn", `{"type":"turn.completed","turn":{"status":"completed","error":{"codex_error_info":"usageLimitExceeded"}}}`, ""},
		{"ambiguous codex error variants",
			`{"type":"error","error":{"codex_error_info":{"usage_limit_exceeded":{},"unexpected":{}}}}`, ""},
		{"raw 429 does not imply retry", `{"type":"error","status_code":429,"error":{"message":"rate limited"}}`, ""},
		{"faked stderr", `{"type":"event_msg","payload":{"message":"usage limit exceeded"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &v); err != nil {
				t.Fatal(err)
			}
			got := structuredWorkflowRecoveryClass04360(v)
			if got != tc.want {
				t.Fatalf("%s class=%q want %q", tc.name, got, tc.want)
			}
		})
	}
}
