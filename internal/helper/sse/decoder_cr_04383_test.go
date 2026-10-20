package sse

import (
	"reflect"
	"testing"
)

// Prodex 0.438.3 runtime_sse_chunk_line_offsets only treats LF as a wire
// line boundary. runtime_sse_line_plan strips trailing CRs *from an LF line*
// but bare CR is not an independent separator. Treating CR as a separator
// could forge a complete quota event from a malformed single data line.
func TestProdex04383SSEOnlyLFSeparatesWireLines(t *testing.T) {
	cases := []struct {
		name, wire string
		want       []string
	}{
		{"CRLF", "data: first\r\ndata: second\r\n\r\n", []string{"first\nsecond"}},
		{"multiple-trailing-CR", "data: first\r\r\n\n", []string{"first"}},
		{"bare-CR-does-not-separate", "data: first\r\rdata: second\n\n", []string{"first\r\rdata: second"}},
		{"bare-CR-at-EOF-is-padding", "data: first\r\r", []string{"first"}},
		{"no-newline-EOF", "data: final", []string{"final"}},
		{"wire-bare-CR-no-event-until-EOF", "data: first\r\rdata: second", []string{"first\r\rdata: second"}},
	}
	for _, tc := range cases {
		for _, chunk := range []int{1, 2, 7, len(tc.wire)} {
			t.Run(tc.name+"/chunk-"+string(rune('a'+min(chunk, 26)-1)), func(t *testing.T) {
				decoder := NewDecoder(4096)
				var got []string
				for pos := 0; pos < len(tc.wire); pos += chunk {
					for _, data := range decoder.Feed([]byte(tc.wire[pos:min(pos+chunk, len(tc.wire))])) {
						got = append(got, string(data))
					}
				}
				for _, data := range decoder.Finish() {
					got = append(got, string(data))
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("CR/LF parity got=%q want=%q", got, tc.want)
				}
			})
		}
	}
}
