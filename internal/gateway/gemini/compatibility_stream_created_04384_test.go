package gemini

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSEUsesUpstreamCreatedBeforeFirstEvent(t *testing.T) {
	for _, fixture := range []struct{ name, first, second, early, late string }{
		{"fixed", "123", "null", "123", "123"},
		{"zero", "0", "null", "0", "0"},
		{"large", "9007199254740993", "null", "9007199254740993", "9007199254740993"},
		{"uint64 maximum", "18446744073709551615", "null", "18446744073709551615", "18446744073709551615"},
		{"updated", "111", "222", "111", "222"},
		{"invalid control", "1.0", "-1", "17", "17"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 17}
			var wire bytes.Buffer
			for _, created := range []string{fixture.first, fixture.second} {
				if err := state.accept(&wire, []byte(`{"created":`+created+`,"choices":[{"delta":{"content":"text"}}]}`)); err != nil {
					t.Fatal(err)
				}
			}
			if err := state.accept(&wire, []byte("[DONE]")); err != nil {
				t.Fatal(err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var timestamps []string
			for _, frame := range frames {
				var event struct {
					CreatedAt json.RawMessage `json:"created_at"`
				}
				if json.Unmarshal(frame, &event) != nil {
					t.Fatalf("invalid event: %s", frame)
				}
				if len(event.CreatedAt) != 0 {
					timestamps = append(timestamps, string(event.CreatedAt))
				}
			}
			want := []string{fixture.early, fixture.early, fixture.late, fixture.late}
			if !reflect.DeepEqual(timestamps, want) {
				t.Fatalf("timestamps=%v, want %v", timestamps, want)
			}
		})
	}
}

func TestProdex04384GeminiSSEErrorDoesNotApplyChunkTimestamp(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 17}
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"created":123,"error":{"message":"synthetic error"}}`)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire.Bytes(), []byte(`"created_at":17`)) || bytes.Contains(wire.Bytes(), []byte("response.created")) {
		t.Fatalf("error precedence changed: %s", wire.Bytes())
	}
}
