package deepseek

import (
	"net/http"
	"testing"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

func TestClassifyDeepSeekErrorBodyMatchesProdexSignals(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		class  providerentity.ErrorClass
		cool   time.Duration
	}{
		{name: "429 message is unstructured", status: http.StatusTooManyRequests, body: `{"error":{"message":"server overloaded"}}`, class: providerentity.ErrorOther},
		{name: "429 reason is structured", status: http.StatusTooManyRequests, body: `{"error":{"reason":"rate_limit_error"}}`, class: providerentity.ErrorRateLimit, cool: time.Minute},
		{name: "best nested code wins", status: http.StatusTooManyRequests, body: `{"error":[{"code":"server_is_overloaded"},{"code":"not_found_error"},{"code":"slow_down"}]}`, class: providerentity.ErrorRateLimit, cool: time.Minute},
		{name: "bare 529 is unclassified", status: 529, body: `{}`, class: providerentity.ErrorOther},
		{name: "529 body text classifies", status: 529, body: `server overloaded`, class: providerentity.ErrorTransient, cool: 10 * time.Second},
		{name: "shared quota extension stays unknown", status: http.StatusBadRequest, body: `{"error":{"code":"usage_limit_reached"}}`, class: providerentity.ErrorOther},
		{name: "SSE data extracts reason", status: http.StatusBadRequest, body: "event: error\ndata: {\"error\":{\"reason\":\"slow_down\"}}\n\n", class: providerentity.ErrorRateLimit, cool: time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := classifyDeepSeekErrorBody(test.status, []byte(test.body))
			if got.Class != test.class || got.Cooldown != test.cool {
				t.Fatalf("classification = %#v, want class %v cooldown %s", got, test.class, test.cool)
			}
		})
	}
}
