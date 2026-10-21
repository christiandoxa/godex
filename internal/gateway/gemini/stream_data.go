package gemini

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"
)

const geminiStreamJSONMaxBytes04384 = 4 << 20

type geminiStreamJSON04384 struct {
	pending []byte
}

func (state *geminiStreamJSON04384) observe(data []byte) ([][]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("stream did not contain valid UTF-8")
	}
	if values, separate := geminiSeparateSSEValues04384(data); separate {
		return values, nil
	}
	fragment := bytes.TrimSpace(data)
	if len(fragment) > geminiStreamJSONMaxBytes04384-len(state.pending) {
		return nil, fmt.Errorf("provider SSE accumulated JSON exceeded safe size limit (%d)", geminiStreamJSONMaxBytes04384)
	}
	state.pending = append(state.pending, fragment...)
	if _, valid := geminiSerdeResponseObject04384(state.pending); !valid {
		return nil, nil
	}
	value := state.pending
	state.pending = nil
	return [][]byte{value}, nil
}

func geminiSSEDone04384(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
			return true
		}
	}
	return false
}

// Joined JSON takes precedence; this fallback validates every separate value
// before any is observed, matching the tagged shared provider reader.
func geminiSeparateSSEValues04384(data []byte) ([][]byte, bool) {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 {
		return nil, false
	}
	var values [][]byte
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if _, valid := geminiSerdeResponseObject04384(line); !valid {
			return nil, false
		}
		values = append(values, line)
	}
	return values, len(values) > 0
}
