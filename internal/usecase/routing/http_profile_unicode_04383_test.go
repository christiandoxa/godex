package routing

import (
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged 0.438.3 Mojo runtime_error_contains folds ASCII capitals ONLY.
// Go strings.ToLower maps U+212A KELVIN SIGN to 'k', which can forge
// deactivated_workspace or workspace-credit quota markers on HTTP 402/403.
func TestProdex04383HTTPProfileAndQuotaTextsDoNotUnicodeFold(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		profile, quota bool
	}{
		{"profile-ascii", `{"error":{"code":"deactivated_workspace"}}`, true, false},
		{"profile-uppercase", `{"error":{"code":"DEACTIVATED_WORKSPACE"}}`, true, false},
		{"profile-kelvin", `{"error":{"code":"deactivated_worKspace"}}`, false, false},
		{"profile-kelvin-uppercase", `{"error":{"code":"DEACTIVATED_WORKSPACE"}}`, false, false},
		{"quota-ascii", `{"error":{"message":"Your workspace is out of credits"}}`, false, true},
		{"quota-uppercase", `{"error":{"message":"YOUR WORKSPACE IS OUT OF CREDITS"}}`, false, true},
		{"quota-kelvin", `{"error":{"message":"Your worKspace is out of credits"}}`, false, false},
		{"quota-kelvin-code", `{"error":{"code":"worKspace_member_credits_depleted"}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAIProfileUnavailable([]byte(tc.body)); got != tc.profile {
				t.Fatalf("profile unavailable for %q=%t want=%t", tc.body, got, tc.profile)
			}
			if got := openAIWorkspaceQuotaResponse([]byte(tc.body)); got != tc.quota {
				t.Fatalf("workspace quota for %q=%t want=%t", tc.body, got, tc.quota)
			}
		})
	}
}

// Check actual Responses routing policy: a 403 with confusable text must
// pass through, whereas recognized ASCII signals authorize owner rotation.
func TestProdex04383HTTP403KelvinTextDoesNotRotateReadyOwner(t *testing.T) {
	for _, tc := range []struct {
		name, body            string
		retry, profile, quota bool
	}{
		{"profile-ascii", `{"error":{"code":"deactivated_workspace"}}`, true, true, false},
		{"profile-kelvin", `{"error":{"code":"deactivated_worKspace"}}`, false, false, false},
		{"quota-ascii", `{"error":{"message":"Your workspace is out of credits"}}`, true, false, true},
		{"quota-kelvin", `{"error":{"message":"Your worKspace is out of credits"}}`, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newProdex04356Router(t, &prodex04356Gateway{}, prodex04356Accounts())
			response := &proxymodel.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader(tc.body))}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			pending.close()
			if retry := outcome.kind == responseRetry; retry != tc.retry || outcome.profileUnavailable != tc.profile || outcome.quota != tc.quota {
				t.Fatalf("HTTP403 Kelvin policy %s unexpected: %+v", tc.name, outcome)
			}
		})
	}
}

func TestProdex04383HTTP402QuotaCodeRequiresExactASCII(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		quota      bool
	}{
		{"canonical", "workspace_member_credits_depleted", true},
		{"uppercase", "WORKSPACE_MEMBER_CREDITS_DEPLETED", true},
		{"kelvin", "worKspace_member_credits_depleted", false},
		{"padded", "workspace_member_credits_depleted ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"error":{"code":"` + tc.code + `"}}`)
			if got := isQuotaResponse(body); got != tc.quota {
				t.Fatalf("quota code %q got=%t want=%t", tc.code, got, tc.quota)
			}
		})
	}
}
