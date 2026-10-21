package routing

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The tagged quota_reset_at_from_json_message first serializes a complete
// serde_json::Value, then passes that CANONICAL representation through a
// 16MiB capped Mojo rich_view_valid. A huge value cannot mint a quota reset;
// large raw JSON whitespace is removed by serde serialization and must not
// by itself cause rejection.
func TestProdex04383QuotaJSONCanonicalSizeNotRawSizeBoundsReset(t *testing.T) {
	const limit = 16 << 20
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name, payload string
		want          int64
	}{
		{"under-limit-ASCII", `{"resets_at":2000000000,"padding":"` + strings.Repeat("x", limit-2048) + `"}`, 2_000_000_000},
		{"over-limit-ASCII", `{"resets_at":2000000000,"padding":"` + strings.Repeat("x", limit+2048) + `"}`, 0},
		{"over-limit-insignificant-whitespace", `{"resets_at":2000000000,` + strings.Repeat(" ", limit+2048) + `"padding":"ok"}`, 2_000_000_000},
		{"over-limit-number-canonicalizes-short", `{"resets_at":2000000000,"padding":1.` + strings.Repeat("0", limit+250) + `}`, 2_000_000_000},
		{"unicode-line-separator-not-Go-escaped", `{"resets_at":2000000000,"padding":"` + strings.Repeat("\u2028", 3<<20) + `"}`, 2_000_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset := quotaResetAtFromMessage([]byte(tc.payload), now)
			if reset != tc.want {
				t.Fatalf("quota large canonical JSON bytes=%d result=%d want=%d", len(tc.payload), reset, tc.want)
			}
		})
	}
}

// Compare the size estimator against serde_json 1.0.151's actual canonical
// output, especially Ryu's fixed/scientific threshold and integer fallback.
func TestProdex04383QuotaJSONCanonicalNumberSizesMatchSerdeRyu(t *testing.T) {
	for _, tc := range []struct{ source, canonical string }{
		{"1.0", "1.0"}, {"1.0000000000000000000", "1.0"},
		{"1e-5", "0.00001"}, {"1e-6", "1e-6"}, {"1e-7", "1e-7"},
		{"1e+6", "1000000.0"}, {"1e+15", "1000000000000000.0"},
		{"1e+16", "1e+16"}, {"1e+20", "1e+20"}, {"1e+308", "1e+308"},
		{"1.23456789e+7", "12345678.9"}, {"1.23456789e-5", "0.0000123456789"},
		{"18446744073709551615", "18446744073709551615"},
		{"18446744073709551616", "1.8446744073709552e+19"},
		{"-0", "-0.0"}, {"-1e-7", "-1e-7"}, {"1e-999", "0.0"},
		// serde_json's u64-significand + literal-pow10 float parser is
		// observably different from Go strconv.ParseFloat. Each expected
		// spelling comes from the exact pinned serde_json 1.0.151 crate.
		{"-9e126", "-9e+126"},
		{"329e59", "3.29e+61"},
		{"28443e-57", "2.8443e-53"},
		{"-93e-155", "-9.3e-154"},
		{"1516712742407918e-134", "1.516712742407918e-119"},
		{"3223604393415311829e59", "3.2236043934153116e+77"},
		{"485312309e-25", "4.8531230899999996e-17"},
		{"-980343254708203970295145288160.11111111111", "-9.803432547082039e+29"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			got, ok := quotaCanonicalNumberSize04383(json.Number(tc.source))
			if !ok || got != len(tc.canonical) {
				t.Fatalf("serde Ryu %s estimated length=%d ok=%t want=%d (%s)", tc.source, got, ok, len(tc.canonical), tc.canonical)
			}
		})
	}
}

// rich_view_valid(...,16MiB) accepts exactly the maximum but rejects one
// byte more. The original JSON is valid in both cases: only the tagged
// canonical serde_json::Value length controls the quota reset.
func TestProdex04383QuotaJSONCanonicalExactBoundary(t *testing.T) {
	const limit = quotaCanonicalJSONLimit04383
	const prototype = `{"padding":"","resets_at":2000000000}`
	for _, tc := range []struct {
		name   string
		change int
		want   int64
	}{
		{"exact-limit", 0, 2000000000},
		{"one-byte-over", 1, 0},
		{"one-byte-under", -1, 2000000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"padding":"` + strings.Repeat("x", limit-len(prototype)+tc.change) + `","resets_at":2000000000}`
			if len(payload) != limit+tc.change {
				t.Fatalf("invalid test boundary: %d", len(payload))
			}
			got := quotaResetAtFromMessage([]byte(payload), time.Unix(1_000_000_000, 0))
			if got != tc.want {
				t.Fatalf("serde canonical length=%d yielded reset=%d want=%d", len(payload), got, tc.want)
			}
		})
	}
}

// Large raw decimal spellings may canonicalize to tiny f64 values. The
// syntactically valid input must be accepted based on the canonical form,
// not rejected by a raw-byte or Go JSON-number-length cap.
func TestProdex04383QuotaJSONRawFloatCanonicalCollapse(t *testing.T) {
	const hugeDecimalZeros = quotaCanonicalJSONLimit04383 + 250
	event := `{"resets_at":2000000000,"ignored":1.` + strings.Repeat("0", hugeDecimalZeros) + `}`
	if got := quotaResetAtFromMessage([]byte(event), time.Unix(1_000_000_000, 0)); got != 2000000000 {
		t.Fatalf("pinned serde 1.0.151 decimal collapse changed quota reset=%d", got)
	}
}
