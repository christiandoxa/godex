package sse

import (
	"strings"
	"testing"
)

func TestStrictDecoderCountsPhysicalLineBytesAtLFAndEOF(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		limit      int
		wantError  bool
	}{
		{"exact-LF", "data: 0\n\n", 8, false},
		{"over-LF", "data: 00\n\n", 8, true},
		{"exact-CRLF", "data: 0\r\n\r\n", 9, false},
		{"over-CRLF", "data: 0\r\n\r\n", 8, true},
		{"exact-EOF", "data: 00", 8, false},
		{"over-EOF", "data: 000", 8, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewStrictDecoder(tc.limit)
			var values [][]byte
			for _, character := range []byte(tc.wire) {
				values = append(values, decoder.Feed([]byte{character})...)
			}
			values = append(values, decoder.Finish()...)
			if (decoder.Err() != nil) != tc.wantError || !tc.wantError && len(values) != 1 || tc.wantError && len(values) != 0 {
				t.Fatalf("strict physical line=%q error=%v values=%q", tc.wire, decoder.Err(), values)
			}
		})
	}
}

func TestStrictDecoderCountsDataAndBOMBytesAndNeverRecoversAfterFailure(t *testing.T) {
	for _, tc := range []struct{ name, wire, want string }{
		{"event-exact", "data:12345\ndata:12345\n\n", ""},
		{"event-over", "data:12345\ndata:123456\n\n", "provider SSE event exceeded safe size limit (12)"},
		{"BOM-counted", "data:\xef\xbb\xbf00\ndata:123456\n\n", "provider SSE event exceeded safe size limit (12)"},
		{"invalid-comment", ":\xff\n\n", "stream did not contain valid UTF-8"},
		{"invalid-ignored-field", "id:\xff\n\n", "stream did not contain valid UTF-8"},
		{"invalid-tail", "data:\xff", "stream did not contain valid UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewStrictDecoder(12)
			values := decoder.Feed([]byte(tc.wire))
			values = append(values, decoder.Finish()...)
			if tc.want == "" {
				if decoder.Err() != nil || len(values) != 1 {
					t.Fatalf("exact event bound rejected: values=%q err=%v", values, decoder.Err())
				}
				return
			}
			if decoder.Err() == nil || decoder.Err().Error() != tc.want || len(values) != 0 {
				t.Fatalf("strict failure=%v values=%q want=%q", decoder.Err(), values, tc.want)
			}
			if output := decoder.Feed([]byte("data:0\n\n")); len(output) != 0 || len(decoder.Finish()) != 0 {
				t.Fatal("terminal decoder recovered after unsafe input")
			}
		})
	}
	decoder := NewStrictDecoder(12)
	values := decoder.Feed([]byte("data:0\n\n:" + strings.Repeat("a", 12)))
	if len(values) != 1 || string(values[0]) != "0" || decoder.Err() == nil {
		t.Fatal("later error lost an earlier complete event")
	}
}
