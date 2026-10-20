package routing

import (
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged 0.438.3 HTTP 402/403 quota evidence uses Mojo's raw JSON member
// and code-token comparisons. Go json.Unmarshal decodes escaped key/value
// spellings that cannot authorize quota rotation in the native raw scanner.
func TestProdex04383HTTP402403RawQuotaCodesDoNotBorrowEscapedMembers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  bool
	}{
		{"canonical-402", 402, `{"error":{"code":"insufficient_quota"}}`, true},
		{"scalar-error-402", 402, `{"error":"insufficient_quota"}`, true},
		{"escaped-scalar-error-402", 402, `{"error":"insufficient_\u0071uota"}`, false},
		{"canonical-uppercase-403", 403, `{"error":{"code":"INSUFFICIENT_QUOTA"}}`, true},
		{"escaped-key-402", 402, `{"error":{"\u0063ode":"insufficient_quota"}}`, false},
		{"escaped-value-402", 402, `{"error":{"code":"insufficient_\u0071uota"}}`, false},
		{"escaped-key-403", 403, `{"error":{"\u0063ode":"insufficient_quota"}}`, false},
		{"nested-control-403", 403, strings.Repeat(`{"wrapper":`, 4) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 4), true},
		{"overdepth-403", 403, strings.Repeat(`{"wrapper":`, 34) + `{"code":"insufficient_quota"}` + strings.Repeat(`}`, 34), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newProdex04356Router(t, &prodex04356Gateway{}, prodex04356Accounts())
			response := &proxymodel.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}},
				Body: io.NopCloser(strings.NewReader(tc.body))}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			pending.close()
			if retry := outcome.kind == responseRetry; retry != tc.retry || outcome.quota != tc.retry {
				t.Fatalf("HTTP %d raw code for %s yielded %+v (wantRetry=%t)", tc.status, tc.name, outcome, tc.retry)
			}
		})
	}
}
