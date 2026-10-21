package sse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

const accumulatedJSONMaxBytes = 4 << 20

// JSONFrames retains partial JSON between SSE events. Complete joined values
// and fully validated separate data lines take precedence over pending JSON.
type JSONFrames struct {
	pending []byte
}

// Pending reports an unfinished JSON segment awaiting another SSE event.
func (state *JSONFrames) Pending() bool { return len(state.pending) > 0 }

// Observe returns complete JSON data values and retains unfinished segments.
func (state *JSONFrames) Observe(data []byte) ([][]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("stream did not contain valid UTF-8")
	}
	fragment := bytes.TrimSpace(data)
	if validJSONFrame(fragment) {
		return [][]byte{fragment}, nil
	}
	if values, separate := separateJSONFrames(data); separate {
		return values, nil
	}
	if len(fragment) > accumulatedJSONMaxBytes-len(state.pending) {
		return nil, fmt.Errorf("provider SSE accumulated JSON exceeded safe size limit (%d)", accumulatedJSONMaxBytes)
	}
	state.pending = append(state.pending, fragment...)
	if !validJSONFrame(state.pending) {
		return nil, nil
	}
	value := state.pending
	state.pending = nil
	return [][]byte{value}, nil
}

// JSONDone recognizes a marker on any data line after the reader's UTF-8 gate.
func JSONDone(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
			return true
		}
	}
	return false
}

func separateJSONFrames(data []byte) ([][]byte, bool) {
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
		if !validJSONFrame(line) {
			return nil, false
		}
		values = append(values, line)
	}
	return values, len(values) > 0
}

func validJSONFrame(data []byte) bool {
	return json.Valid(data) && jsonutil.CheckSerdeBoundary(string(data)) == nil && jsonutil.CheckSerdeNumberTokens(data)
}
