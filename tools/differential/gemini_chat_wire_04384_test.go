package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatOracleRejectsEquivalentEscapedWire(t *testing.T) {
	good := geminiChatSSEFixture043711("resp_deepseek_fixture", "msg_gemini_fixture", 111)
	if _, err := geminiChatSSEProjection043711(good); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ literal, escaped string }{
		{"<", `\u003c`}, {">", `\u003e`}, {"&", `\u0026`},
		{"\u2028", `\u2028`}, {"\u2029", `\u2029`},
	} {
		t.Run(tc.escaped, func(t *testing.T) {
			mutated := strings.Replace(good, tc.literal, tc.escaped, 1)
			left, right := sse.NewDecoder(1<<20).Feed([]byte(good)), sse.NewDecoder(1<<20).Feed([]byte(mutated))
			if mutated == good || len(left) != len(right) {
				t.Fatal("escape mutation did not preserve the fixture framing")
			}
			for index := range left {
				var first, next any
				if json.Unmarshal(left[index], &first) != nil || json.Unmarshal(right[index], &next) != nil {
					t.Fatal("escape mutation produced malformed JSON")
				}
				firstJSON, _ := json.Marshal(first)
				nextJSON, _ := json.Marshal(next)
				if string(firstJSON) != string(nextJSON) {
					t.Fatal("escape mutation changed the decoded event")
				}
			}
			if _, err := geminiChatSSEProjection043711(mutated); err == nil {
				t.Fatal("oracle accepted a semantic-equivalent byte mutation")
			}
		})
	}
	for _, mutated := range []string{
		strings.ReplaceAll(good, "\r\n", "\n"),
		strings.Replace(good, `"sequence_number":0`, `"sequence_number": 0`, 1),
		strings.Replace(good, "data: ", "data:", 1),
	} {
		if _, err := geminiChatSSEProjection043711(mutated); err == nil {
			t.Fatal("oracle accepted a framing/whitespace byte mutation")
		}
	}
}
