package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Source Prodex 0.438.4 runtime_request_target.mojo validates the raw URL
// before local-rewrite provider dispatch. Percent-encoded spaces, Unicode,
// dot segments and separators in the path must reject HTTP 400 even if a
// catalog lookup on the decoded path would otherwise return HTTP 404/200.
func TestProdex04384ProxyRequestTargetUsesCompiledMojoPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		valid        bool
	}{
		{"root", "/", true},
		{"models", "/backend-api/godex/models/gemini-3.1-pro-preview", true},
		{"query", "/backend-api/godex/models?limit=10", true},
		{"query-encoded-byte", "/backend-api/godex/models?opaque=%FF", true},
		{"query-percent-escaped-hash", "/backend-api/godex/models?value=%23", true},
		{"query-raw-backslash", "/backend-api/godex/models?value=\\", true},
		{"raw-space", "/backend-api/godex/models/ gemini", false},
		{"encoded-space", "/backend-api/godex/models/%20gemini", false},
		{"encoded-unicode", "/backend-api/godex/models/gemini-%C4%B0", false},
		{"raw-unicode", "/backend-api/godex/models/gemini-İ", false},
		{"raw-dot", "/backend-api/godex/./models", false},
		{"raw-dotdot", "/backend-api/godex/../models", false},
		{"encoded-dot", "/backend-api/godex/models/%2e", false},
		{"encoded-slash", "/backend-api/godex/models/%2Fmodel", false},
		{"encoded-percent", "/backend-api/godex/models/%25model", false},
		{"encoded-query", "/backend-api/godex/models/%3Fmodel", false},
		{"encoded-hash", "/backend-api/godex/models/%23model", false},
		{"encoded-backslash", "/backend-api/godex/models/%5cmodel", false},
		{"double-leading-slash", "//backend-api/godex/models", false},
		{"double-internal-slash", "/backend-api//godex/models", false},
		{"raw-fragment", "/backend-api/godex/models#frag", false},
		{"raw-control", "/backend-api/godex/models\x00", false},
		{"malformed-percent", "/backend-api/godex/models/%X0", false},
		{"max-size-8192", "/" + strings.Repeat("a", 8191), true},
		{"over-max-8193", "/" + strings.Repeat("a", 8192), false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if valid := validProdexRequestTarget04384(tc.target); valid != tc.valid {
				t.Fatalf("Mojo request-target %q valid=%t want=%t", tc.name, valid, tc.valid)
			}
		})
	}
}

func TestProdex04384ProxyRejectsUnsafeEncodedModelPathBeforeRouter(t *testing.T) {
	for _, tc := range []struct{ name, target string }{
		{"space", "/backend-api/godex/models/%20default%20"},
		{"unicode", "/backend-api/godex/models/gemini-prev%C4%B0ew"},
		{"escaped-slash", "/backend-api/godex/models/%2fgemini"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No router is deliberately configured. These requests must terminate
			// before touching accounts, credentials, the provider, or admission.
			proxy := &Proxy{}
			req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			req.RequestURI = tc.target
			req.URL.Path = tc.target
			writer := httptest.NewRecorder()
			proxy.ServeHTTP(writer, req)
			if writer.Code != http.StatusBadRequest {
				t.Fatalf("unsafe request %s returned HTTP %d", tc.target, writer.Code)
			}
			var body struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "invalid_request_target" || body.Error.Message != "request target is invalid" {
				t.Fatalf("unsafe request %s changed response: %s", tc.target, writer.Body.String())
			}
		})
	}
}
