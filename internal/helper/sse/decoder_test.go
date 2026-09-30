package sse

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecoderFramingAndChunkBoundaries(t *testing.T) {
	stream := "\xef\xbb\xbf: comment\r\nevent: message\r\ndata: {\r\ndata: \"id\":\"late\"}\r\n\r\ndata: second\r\rdata\n\n"
	for _, chunkSize := range []int{1, 2, 7, len(stream)} {
		decoder := NewDecoder(128)
		var got []string
		for start := 0; start < len(stream); start += chunkSize {
			for _, data := range decoder.Feed([]byte(stream[start:min(start+chunkSize, len(stream))])) {
				got = append(got, string(data))
			}
		}
		if !reflect.DeepEqual(got, []string{"{\n\"id\":\"late\"}", "second", ""}) {
			t.Fatalf("chunk size %d: %q", chunkSize, got)
		}
	}
}

func TestDecoderSkipsOversizedEventsAndRecovers(t *testing.T) {
	decoder := NewDecoder(16)
	for _, chunk := range []string{"data: " + strings.Repeat("x", 1000), "\ndata: hidden\n\n", "data: 123456789\ndata: 123456789\n\n"} {
		if events := decoder.Feed([]byte(chunk)); len(events) != 0 || len(decoder.line) > 16 || len(decoder.data) > 16 {
			t.Fatalf("oversized event escaped bound: %q", events)
		}
	}
	events := decoder.Feed([]byte("data: recovered\n\n"))
	if len(events) != 1 || string(events[0]) != "recovered" {
		t.Fatalf("decoder did not recover: %q", events)
	}
}
