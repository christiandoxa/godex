package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func geminiCompactStreamSeed04384(scenario string) bool {
	return scenario == "compact-owner-stream" || scenario == "compact-owner-stream-eof" || scenario == "compact-owner-stream-unbound" || geminiCompactReasoningStream04384(scenario) != ""
}

func geminiCompactReasoningStream04384(scenario string) string {
	switch scenario {
	case "compact-owner-stream-reasoning":
		return "stream-chat-reasoning"
	case "compact-owner-stream-reasoning-only":
		return "stream-chat-reasoning-only"
	default:
		return ""
	}
}

func geminiCompactStreamInput04384(scenario string, seed exchange) ([]byte, error) {
	body := []byte(geminiCompactInput04384(scenario))
	if !geminiCompactStreamSeed04384(scenario) || scenario == "compact-owner-stream-unbound" {
		return body, nil
	}
	decoder := sse.NewStrictDecoder(4 << 20)
	frames := append(decoder.Feed([]byte(seed.Body)), decoder.Finish()...)
	if decoder.Err() != nil {
		return nil, errors.New("invalid streamed Compact seed")
	}
	for _, frame := range frames {
		var event struct {
			Type     string              `json:"type"`
			Response struct{ ID string } `json:"response"`
		}
		if json.Unmarshal(frame, &event) != nil || event.Type != "response.created" || !strings.HasPrefix(event.Response.ID, "resp_deepseek_") {
			continue
		}
		var request map[string]any
		if json.Unmarshal(body, &request) != nil {
			return nil, errors.New("invalid Compact fixture input")
		}
		request["previous_response_id"] = event.Response.ID
		return json.Marshal(request)
	}
	return nil, errors.New("streamed Compact seed lacks its public response ID")
}

func geminiCompactStreamSeedExchange04384(scenario string) exchange {
	body := geminiChatSSEFixture043711("resp_deepseek_fixture", "msg_gemini_fixture", 111)
	if reasoning := geminiCompactReasoningStream04384(scenario); reasoning != "" {
		body = geminiReasoningStreamFixture04384(reasoning, "resp_deepseek_fixture", "msg_gemini_fixture", 111)
	}
	if scenario == "compact-owner-stream-eof" {
		body = geminiChatEOFFixture04384("resp_deepseek_fixture", "msg_gemini_fixture", 111)
	}
	return exchange{Status: 200, Body: body, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}}
}

func geminiCompactStreamSeedValid04384(scenario string, seed *exchange) bool {
	if seed == nil || seed.Seed != nil || seed.Status != 200 || seed.Headers.Get("Content-Type") != "text/event-stream; charset=utf-8" {
		return false
	}
	if scenario == "compact-owner-stream-eof" {
		return geminiChatEOFWireValid04384(seed.Body)
	}
	if reasoning := geminiCompactReasoningStream04384(scenario); reasoning != "" {
		return geminiReasoningStreamWireValid04384(reasoning, seed.Body)
	}
	_, err := geminiChatSSEProjection043711(seed.Body)
	return err == nil
}
