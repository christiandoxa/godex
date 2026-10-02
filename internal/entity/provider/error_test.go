package provider

import (
	"testing"
	"time"
)

func TestClassifyErrorMatchesProdexCorePolicy(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		class  ErrorClass
		cool   time.Duration
	}{
		{"auth401", 401, `{"error":{"code":"insufficient_quota"}}`, ErrorAuth, 0},
		{"auth403", 403, `{"error":{"code":"quota_exhausted"}}`, ErrorAuth, 0},
		{"bare429", 429, `{"error":{"message":"too many requests"}}`, ErrorOther, 0},
		{"rate429", 429, `{"error":{"code":"rate_limit_exceeded"}}`, ErrorRateLimit, time.Minute},
		{"rate error", 429, `{"error":{"type":"rate_limit_error"}}`, ErrorRateLimit, time.Minute},
		{"quota429", 429, `{"error":{"type":"organization_spend_limit_exceeded"}}`, ErrorQuota, 5 * time.Minute},
		{"notfound", 404, `{"error":{"message":"missing"}}`, ErrorNotFound, 0},
		{"modelcode", 500, `{"error":{"code":"model_not_supported"}}`, ErrorNotFound, 0},
		{"not found code", 500, `{"error":{"type":"not_found_error"}}`, ErrorNotFound, 0},
		{"overloaded code", 400, `{"error":{"type":"overloaded_error"}}`, ErrorTransient, 10 * time.Second},
		{"server overloaded code", 400, `{"error":{"code":"server_is_overloaded"}}`, ErrorTransient, 10 * time.Second},
		{"transient", 503, `{}`, ErrorTransient, 10 * time.Second},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			got := ClassifyError(fixture.status, []byte(fixture.body))
			if got.Class != fixture.class || got.Cooldown != fixture.cool {
				t.Fatalf("classification = %#v", got)
			}
		})
	}
}

func TestRetryEligibilityMatchesProdexTransitionPolicy(t *testing.T) {
	if RetryableAcrossModels(ErrorAuth) || !RetryableAcrossModels(ErrorNotFound) || !RetryableAcrossModels(ErrorTransient) {
		t.Fatal("model retry policy drift")
	}
	if !RetryableAcrossCredentials(ErrorAuth) || RetryableAcrossCredentials(ErrorNotFound) || RetryableAcrossCredentials(ErrorOther) {
		t.Fatal("credential retry policy drift")
	}
}
