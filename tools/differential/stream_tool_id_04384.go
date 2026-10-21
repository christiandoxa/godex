package main

import (
	"encoding/json"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
	"github.com/google/uuid"
)

func streamToolIDFixture04384(scenario string) (id string, known bool) {
	switch scenario {
	case "sse-tool-id-missing":
		return "call_deepseek_fixture", true
	case "sse-tool-id-empty":
		return "", true
	case "sse-tool-id-padded":
		return " call-preserved ", true
	}
	return "", false
}

func streamToolIDWire04384(scenario string) string {
	id, known := streamToolIDFixture04384(scenario)
	if !known {
		return ""
	}
	quoted, _ := json.Marshal(id)
	signature, _ := json.Marshal(streamedToolSignature04384)
	wire := strings.ReplaceAll(canonicalToolSignatureWire04384(), `,"gemini_thought_signature":`+string(signature), "")
	return strings.ReplaceAll(wire, `"call-toolargs"`, string(quoted))
}

func equivalentStreamToolIDSSE04384(left, right string) bool {
	for _, scenario := range []string{"sse-tool-id-missing", "sse-tool-id-empty", "sse-tool-id-padded"} {
		if validStreamToolIDWire04384(scenario, left) && validStreamToolIDWire04384(scenario, right) {
			return true
		}
	}
	return false
}

func validStreamToolIDWire04384(scenario, wire string) bool {
	if scenario != "sse-tool-id-missing" {
		want := streamToolIDWire04384(scenario)
		return want != "" && wire == want
	}
	decoder := sse.NewDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(wire)), decoder.Finish()...)
	var added struct {
		Item struct {
			CallID string `json:"call_id"`
		}
	}
	if len(frames) != 5 || json.Unmarshal(frames[1], &added) != nil {
		return false
	}
	id := added.Item.CallID
	parsed, err := uuid.Parse(strings.TrimPrefix(id, "call_deepseek_"))
	if err != nil || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || id != "call_deepseek_"+parsed.String() {
		return false
	}
	wire = strings.ReplaceAll(wire, `"call_id":"`+id+`"`, `"call_id":"call_deepseek_fixture"`)
	return wire == streamToolIDWire04384(scenario)
}
