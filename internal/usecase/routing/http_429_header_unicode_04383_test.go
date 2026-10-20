package routing

import (
	"io"
	"net/http"
	"strings"
	"testing"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged 0.438.3 rate_limit_header_matches compares raw bytes and folds
// ASCII uppercase ONLY. Go strings.ToLower performs Unicode lowercasing:
// U+212A KELVIN SIGN becomes ASCII 'k' and can forge a workspace quota tag.
func TestProdex04383HTTP429HeaderClassificationDoesNotUnicodeFold(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		class       providerentity.ErrorClass
		matched     bool
	}{
		{"quota-control", "workspace_owner_credits_depleted", providerentity.ErrorQuota, true},
		{"quota-uppercase", "WORKSPACE_OWNER_CREDITS_DEPLETED", providerentity.ErrorQuota, true},
		{"quota-trim", " workspace_owner_credits_depleted \t", providerentity.ErrorQuota, true},
		{"quota-kelvin", "worKspace_owner_credits_depleted", providerentity.ErrorOther, false},
		{"quota-kelvin-uppercase", "WORKSPACE_OWNER_CREDITS_DEPLETED", providerentity.ErrorOther, false},
		{"quota-wrong-name", "workspace_owner_credits_depleted_", providerentity.ErrorOther, false},
		{"rate-control", "RATE_LIMIT_REACHED", providerentity.ErrorRateLimit, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{"X-Codex-Rate-Limit-Reached-Type": {tc.value}}
			got, matched := openAI429HeaderClassification(header)
			if matched != tc.matched || (matched && got.Class != tc.class) {
				t.Fatalf("rate-limit header %q class=%v matched=%t wantClass=%v matched=%t", tc.value, got.Class, matched, tc.class, tc.matched)
			}
		})
	}
}

// The hint header must not override a valid raw 429 rate-limit body when
// its Unicode spelling does not match the exact tagged ASCII quota class.
func TestProdex04383HTTP429KelvinQuotaHeaderKeepsRateBody(t *testing.T) {
	header := http.Header{"X-Codex-Rate-Limit-Reached-Type": {"worKspace_owner_credits_depleted"}}
	classification, ok := openAI429HeaderClassification(header)
	if ok && classification.Class == providerentity.ErrorQuota {
		t.Fatal("Unicode-confusable quota header stole authoritative rate class")
	}
	body := []byte(`{"error":{"code":"rate_limit_exceeded"}}`)
	if fromBody := openAI429Classification(body); fromBody.Class != providerentity.ErrorRateLimit {
		t.Fatalf("valid rate body lost its classification: %v", fromBody.Class)
	}
}

// A Unicode-confusable header cannot turn an otherwise nonretryable 429
// invalid_prompt into quota rotation. This checks the actual HTTP 429
// candidate policy, not just the header helper's matched bit.
func TestProdex04383HTTP429KelvinHintDoesNotForceRetry(t *testing.T) {
	router := newProdex04356Router(t, &prodex04356Gateway{}, prodex04356Accounts())
	for _, tc := range []struct {
		name, hint   string
		retry, quota bool
	}{
		{"canonical-quota", "workspace_owner_credits_depleted", true, true},
		{"uppercase-quota", "WORKSPACE_OWNER_CREDITS_DEPLETED", true, true},
		{"kelvin-not-quota", "worKspace_owner_credits_depleted", false, false},
		{"kelvin-uppercase-not-quota", "WORKSPACE_OWNER_CREDITS_DEPLETED", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &proxymodel.Response{StatusCode: http.StatusTooManyRequests,
				Header: http.Header{"X-Codex-Rate-Limit-Reached-Type": {tc.hint}},
				Body:   io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_prompt"}}`))}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			pending.close()
			if retry := outcome.kind == responseRetry; retry != tc.retry || outcome.quota != tc.quota {
				t.Fatalf("HTTP429 confusable hint %q changed candidate routing: %+v", tc.hint, outcome)
			}
		})
	}
}
