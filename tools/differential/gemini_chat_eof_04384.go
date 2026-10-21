package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func geminiChatStreamScenario04384(scenario string) bool {
	return scenario == "stream-chat" || scenario == "stream-chat-eof" || scenario == "stream-chat-error" || scenario == "stream-chat-data-lines" || scenario == "stream-chat-json-fragments" || geminiProviderBOMScenario04384(scenario)
}

func geminiChatEOFFixture04384(responseID, itemID string, epoch int64) string {
	success := geminiChatSSEFixture043711(responseID, itemID, epoch)
	prefix := strings.SplitN(success, "event: response.output_item.done\r\n", 2)[0]
	id, _ := json.Marshal(responseID)
	return prefix + fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":\"unexpected end of stream\"},\"id\":%s},\"sequence_number\":4,\"type\":\"response.failed\"}\r\n\r\n", epoch, id)
}

func geminiChatEOFWireValid04384(wire string) bool {
	decoder := sse.NewDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(wire)), decoder.Finish()...)
	if len(frames) != 5 {
		return false
	}
	var created, added struct {
		CreatedAt int64               `json:"created_at"`
		Response  struct{ ID string } `json:"response"`
		Item      struct{ ID string } `json:"item"`
	}
	if json.Unmarshal(frames[0], &created) != nil || json.Unmarshal(frames[1], &added) != nil ||
		created.CreatedAt <= 0 || !strings.HasPrefix(created.Response.ID, "resp_deepseek_") || !strings.HasPrefix(added.Item.ID, "msg_gemini_") {
		return false
	}
	return wire == geminiChatEOFFixture04384(created.Response.ID, added.Item.ID, created.CreatedAt)
}

func geminiChatEOFSentinel04384() bool {
	good := geminiChatEOFFixture04384("resp_deepseek_fixture", "msg_gemini_fixture", 111)
	if !geminiChatEOFWireValid04384(good) || !geminiChatEOFWireValid04384(geminiChatEOFFixture04384("resp_deepseek_other", "msg_gemini_other", 999)) {
		return false
	}
	for _, changed := range []string{
		strings.Replace(good, `"created_at":111,"response":{"error"`, `"response":{"error"`, 1),
		strings.Replace(good, `"created_at":111,"response":{"error"`, `"created_at":999,"response":{"error"`, 1),
		strings.Replace(good, "unexpected end of stream", "success", 1),
		strings.Replace(good, "provider_stream_error", "wrong_code", 1),
		strings.Replace(good, `"sequence_number":4`, `"sequence_number":9`, 1),
		strings.Replace(good, "event: response.failed", "event: response.completed", 1),
		strings.ReplaceAll(good, "\r\n", "\n"),
		strings.SplitN(good, "event: response.failed", 2)[0],
	} {
		if changed == good || geminiChatEOFWireValid04384(changed) {
			return false
		}
	}
	return true
}
