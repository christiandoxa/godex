package copilot

import "testing"

func TestCopilotProviderErrorClassificationMatchesRetryPolicy(t *testing.T) {
	tests := []struct {
		status int
		body   string
		class  copilotProviderErrorClass
		retry  bool
	}{
		{401, `{"error":{"code":"insufficient_quota"}}`, copilotErrorAuth, false},
		{404, `{"error":{"code":"quota_exhausted"}}`, copilotErrorQuota, true},
		{500, `{"error":{"code":"model_not_supported"}}`, copilotErrorNotFound, true},
		{503, `{"error":{"message":"backend overloaded"}}`, copilotErrorTransient, true},
		{429, `{"error":{"code":"rate_limit_exceeded"}}`, copilotErrorRateLimit, true},
		{429, `{"error":{"message":"too many requests"}}`, copilotErrorOther, false},
		{429, `{"error":{"code":"model_not_supported"}}`, copilotErrorNotFound, true},
		{429, `{"error":{"type":"invalid_request_error"}}`, copilotErrorOther, false},
	}
	for _, test := range tests {
		body := []byte(test.body)
		if got := classifyCopilotProviderError(test.status, body); got != test.class {
			t.Fatalf("classify(%d,%s) = %d, want %d", test.status, test.body, got, test.class)
		}
		if got := copilotModelRetryAllowed(test.status, body); got != test.retry {
			t.Fatalf("retry(%d,%s) = %t, want %t", test.status, test.body, got, test.retry)
		}
	}
}

func TestCopilotErrorCodesReadNestedJSONAndSSE(t *testing.T) {
	jsonCodes := copilotErrorCodes([]byte(`{"error":{"details":[{"reason":"rate_limit_exceeded"}],"type":"quota_exhausted"}}`))
	if !sameCopilotCodes(jsonCodes, []string{"rate_limit_exceeded", "quota_exhausted"}) {
		t.Fatalf("JSON codes = %#v", jsonCodes)
	}
	sseCodes := copilotErrorCodes([]byte("data: {\"error\":{\"code\":\"model_not_supported\"}}\n\n"))
	if len(sseCodes) != 1 || sseCodes[0] != "model_not_supported" {
		t.Fatalf("SSE codes = %#v", sseCodes)
	}
}

func sameCopilotCodes(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(want))
	for _, value := range want {
		counts[value]++
	}
	for _, value := range got {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}

func TestProdex04356CopilotGeneric429GateIsPrecommitRetryable(t *testing.T) {
	if !copilotProviderRetryPrecommit(copilotErrorRateLimit, 429, []byte("too many requests")) {
		t.Fatal("tagged Copilot generic-429 gate rejected a precommit rate-limit retry")
	}
	if copilotProviderRetryPrecommit(copilotErrorOther, 429, []byte("too many requests")) {
		t.Fatal("bare generic 429 was promoted from provider class Other")
	}
	if !copilotProviderRetryPrecommit(
		copilotErrorQuota,
		429,
		[]byte(`{"error":{"code":"insufficient_quota","detail":"invalid_request"}}`),
	) {
		t.Fatal("structured quota signal lost precedence to generic nonretryable text")
	}
}
