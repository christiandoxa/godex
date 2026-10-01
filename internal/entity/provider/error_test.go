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
		{"quota429", 429, `{"error":{"type":"organization_spend_limit_exceeded"}}`, ErrorQuota, 5 * time.Minute},
		{"notfound", 404, `{"error":{"message":"missing"}}`, ErrorNotFound, 0},
		{"modelcode", 500, `{"error":{"code":"model_not_supported"}}`, ErrorNotFound, 0},
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
