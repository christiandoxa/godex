package sse

import (
	"reflect"
	"testing"
)

// Tagged 0.438.3 Mojo runtime_sse_payload_join strips consecutive UTF-8
// BOMs only at the beginning of an EVENT'S FIRST data value. Its line planner
// does not interpret a BOM-prefixed wire field name as `data`.
func TestProdex04383SSEBOMAppliesToFirstPayloadNotFieldName(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	for _, tc := range []struct {
		name, wire string
		expected   []string
	}{
		{"payload-single", "data: " + bom + `{"type":"response.failed"}` + "\n\n", []string{`{"type":"response.failed"}`}},
		{"payload-multiple", "data: " + bom + bom + `{"type":"response.failed"}` + "\n\n", []string{`{"type":"response.failed"}`}},
		{"wire-field-bom-ignored", bom + "data: " + `{"type":"response.failed"}` + "\n\n", nil},
		{"after-ignored-bom-line", bom + "data: ignored\n\ndata: visible\n\n", []string{"visible"}},
		{"second-event-first-data-bom", "data: first\n\ndata: " + bom + "second\n\n", []string{"first", "second"}},
		{"second-data-line-bom-preserved", "data: first\ndata: " + bom + "second\n\n", []string{"first\n" + bom + "second"}},
		{"no-bom-normal", "data: normal\n\n", []string{"normal"}},
	} {
		for _, chunkSize := range []int{1, 3, len(tc.wire)} {
			t.Run(tc.name+"/chunk-"+string(rune('0'+min(chunkSize, 9))), func(t *testing.T) {
				d := NewDecoder(1024)
				var got []string
				for off := 0; off < len(tc.wire); off += chunkSize {
					for _, payload := range d.Feed([]byte(tc.wire[off:min(off+chunkSize, len(tc.wire))])) {
						got = append(got, string(payload))
					}
				}
				if !reflect.DeepEqual(got, tc.expected) {
					t.Fatalf("BOM wire decoded=%q want=%q", got, tc.expected)
				}
			})
		}
	}
}
