package deepseek

import (
	"bytes"
	"encoding/json"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func (state *deepSeekChatStreamState) observe(data []byte) ([]byte, bool, error) {
	if state.completed {
		return nil, false, nil
	}
	if sse.JSONDone(data) {
		if state.json.Pending() {
			return state.failed("provider_stream_error", "incomplete JSON segment")
		}
		return state.complete()
	}
	if deepSeekValidStreamValue04384(data) && json.Valid(data) {
		return state.observeValue(data)
	}
	values, err := state.json.Observe(data)
	if err != nil {
		return state.failed("provider_stream_error", err.Error())
	}
	var events [][]byte
	for _, value := range values {
		translated, supported, err := state.observeValue(value)
		if err != nil {
			return nil, false, err
		}
		if supported {
			events = append(events, translated)
		}
		if state.completed {
			break
		}
	}
	return bytes.Join(events, nil), len(events) > 0, nil
}
