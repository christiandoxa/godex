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

func TestProdex04361WorkflowRecoveryHonorsProviderStatusBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{
			name: "transport variant with client status is terminal",
			raw:  `{"type":"event_msg","payload":{"type":"error","http_status_code":400,"codex_error_info":"responseStreamDisconnected"}}`,
		},
		{
			name: "transport variant with transient status retries",
			raw:  `{"type":"event_msg","payload":{"type":"error","http_status_code":503,"codex_error_info":"responseStreamDisconnected"}}`,
			want: "transport",
		},
		{
			name: "nested transport status is checked",
			raw:  `{"type":"error","error":{"codex_error_info":{"responseTooManyFailedAttempts":{"httpStatusCode":400}}}}`,
		},
		{
			name: "unexpected status server failure is transient",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 503 Service Unavailable: unavailable","codex_error_info":"other"}}`,
			want: "transport",
		},
		{
			name: "unexpected status auth stays auth",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 401 Unauthorized: Unauthorized","codex_error_info":"other"}}`,
			want: "auth",
		},
		{
			name: "unexpected status quota uses authoritative text",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 403 Forbidden: You've hit your usage limit. Try again later.","codex_error_info":"other"}}`,
			want: "usage_limit",
		},
		{
			name: "unexpected status profile body rotates unavailable profile",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 403 Forbidden: {\"detail\":{\"code\":\"deactivated_workspace\"}} , url: https://example.test","codex_error_info":"other"}}`,
			want: "profile_unavailable",
		},
		{
			name: "bare unexpected rate status is terminal",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 429 Too Many Requests: rate limited","codex_error_info":"other"}}`,
		},
		{
			name: "non-authoritative usage wording is terminal",
			raw:  `{"type":"event_msg","payload":{"type":"error","message":"unexpected status 403 Forbidden: The usage limit has been reached","codex_error_info":"other"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var record map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &record); err != nil {
				t.Fatal(err)
			}
			if got := structuredWorkflowRecoveryClass04360(record); got != tc.want {
				t.Fatalf("class=%q want %q", got, tc.want)
			}
		})
	}
}

func TestProdex04361WorkflowRecoveryRecognizesQuotaCodeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"usage limit code", `{"type":"error","error":{"code":"usage_limit_reached"}}`, "usage_limit"},
		{"provider quota code", `{"type":"error","error":{"code":"insufficient_quota"}}`, "usage_limit"},
		{"quota reason is trimmed and case folded", `{"type":"error","error":{"reason":" RESOURCE_EXHAUSTED "}}`, "usage_limit"},
		{"workspace code in nested error", `{"type":"error","error":{"error":{"type":"workspace_member_credits_depleted"}}}`, "usage_limit"},
		{"rate limit code is not exhausted quota", `{"type":"error","error":{"code":"rate_limit_exceeded"}}`, ""},
		{"quota wording without code is not evidence", `{"type":"error","error":{"message":"insufficient_quota"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var record map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &record); err != nil {
				t.Fatal(err)
			}
			if got := structuredWorkflowRecoveryClass04360(record); got != tc.want {
				t.Fatalf("class=%q want %q", got, tc.want)
			}
		})
	}
}
