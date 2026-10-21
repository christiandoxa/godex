package kiro

import (
	"strings"
	"testing"
)

// Tagged Rust RuntimeKiroAcpEnvelope uses serde_json::from_str on a typed
// struct. Repeated *known* top-level members fail the entire envelope,
// even if the last duplicate would form a plausible ACP notification.
// Duplicate unknown object keys and duplicate nested serde Value keys are
// valid: JSON object maps are last-wins for those fields.
func TestProdex04384KiroACPEnvelopeRejectsTypedDuplicateFields(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"canonical", `{"jsonrpc":"2.0","method":"session/update","params":{}}`, true},
		{"duplicate-method", `{"jsonrpc":"2.0","method":"other","method":"session/update"}`, false},
		{"duplicate-jsonrpc", `{"jsonrpc":"1.0","jsonrpc":"2.0","method":"session/update"}`, false},
		{"duplicate-id", `{"jsonrpc":"2.0","id":null,"id":4}`, false},
		{"duplicate-params", `{"jsonrpc":"2.0","params":{},"params":{"sessionId":"s"}}`, false},
		{"duplicate-result", `{"jsonrpc":"2.0","result":{},"result":{}}`, false},
		{"duplicate-error", `{"jsonrpc":"2.0","error":null,"error":null}`, false},
		{"error-canonical", `{"jsonrpc":"2.0","error":{"code":-32001,"message":"bad"}}`, true},
		{"error-missing-code", `{"jsonrpc":"2.0","error":{"message":"bad"}}`, false},
		{"error-missing-message", `{"jsonrpc":"2.0","error":{"code":-32001}}`, false},
		{"error-null-code", `{"jsonrpc":"2.0","error":{"code":null,"message":"bad"}}`, false},
		{"error-null-message", `{"jsonrpc":"2.0","error":{"code":-32001,"message":null}}`, false},
		{"error-string-code", `{"jsonrpc":"2.0","error":{"code":"-32001","message":"bad"}}`, false},
		{"duplicate-error-code", `{"jsonrpc":"2.0","error":{"code":1,"code":2,"message":"bad"}}`, false},
		{"duplicate-error-message", `{"jsonrpc":"2.0","error":{"code":1,"message":"first","message":"second"}}`, false},
		{"duplicate-error-unknown-valid", `{"jsonrpc":"2.0","error":{"code":1,"message":"bad","extra":1,"extra":2}}`, true},
		{"duplicate-unknown-valid", `{"jsonrpc":"2.0","future":1,"future":2}`, true},
		{"duplicate-nested-value-valid", `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"text":"first"},"content":{"text":"last"}}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, skipped, err := decodeACPLine([]byte(tc.raw))
			if skipped || (err == nil) != tc.valid {
				t.Fatalf("typed Kiro ACP JSON-RPC duplicate parsed=%t skip=%t err=%v want valid=%t", err == nil, skipped, err, tc.valid)
			}
		})
	}
}

// Tagged serde rejects invalid Unicode before any session update is
// projected into Kiro text/tool metadata. Go's JSON parser replaces malformed
// escapes or bytes with U+FFFD and previously accepted these envelopes.
func TestProdex04384KiroACPEnvelopeRejectsInvalidSerdeUnicode(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"valid-pair", `"\ud83d\ude00"`, true},
		{"valid-escaped-backslash", `"\\ud800"`, true},
		{"lone-high-surrogate", `"\ud800"`, false},
		{"lone-low-surrogate", `"\udc00"`, false},
		{"invalid-high-follow", `"\ud800\u0041"`, false},
		{"invalid-UTF8", `"` + string([]byte{0xff}) + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":` + tc.payload + `}}}}`
			_, _, err := decodeACPLine([]byte(body))
			if (err == nil) != tc.valid {
				t.Fatalf("Kiro JSON with malformed Unicode accepted=%t want=%t err=%v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestProdex04384KiroACPEnvelopeRejectsExcessSerdeNesting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		depth int
		valid bool
	}{
		{"normal-nested", 8, true},
		{"excess-nesting", 140, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","params":{"future":` + strings.Repeat(`[`, tc.depth) + `0` + strings.Repeat(`]`, tc.depth) + `}}`
			_, _, err := decodeACPLine([]byte(body))
			if (err == nil) != tc.valid {
				t.Fatalf("Kiro nesting %d accepted=%t want=%t err=%v", tc.depth, err == nil, tc.valid, err)
			}
		})
	}
}

// serde_json default float parsing rejects a near-f64-maximum decimal that
// Go considers finite. A valid long-mantissa neighbor must remain accepted.
func TestProdex04384KiroACPEnvelopeRejectsSerdeOnlyNumericOverflow(t *testing.T) {
	for _, tc := range []struct {
		name, number string
		valid        bool
	}{
		{"finite-max", "1.7976931348623157e308", true},
		{"serde-overflow-go-finite", "1.7976931348623158e308", false},
		{"valid-long-mantissa", "1.79769313486231580e308", true},
		{"obvious-overflow", "1e309", false},
		{"small-normal", "-1.2e-50", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello"}},"unrelated":` + tc.number + `}}`
			_, _, err := decodeACPLine([]byte(payload))
			if (err == nil) != tc.valid {
				t.Fatalf("serde numeric JSON validity for %s accepted=%t want=%t err=%v", tc.number, err == nil, tc.valid, err)
			}
		})
	}
}
