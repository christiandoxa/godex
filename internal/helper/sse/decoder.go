// Package sse decodes bounded event data without changing stream framing.
package sse

import "bytes"

type Decoder struct {
	limit     int
	line      []byte
	data      []byte
	oversized bool
	skipLF    bool
}

func NewDecoder(limit int) *Decoder {
	return &Decoder{limit: max(1, limit)}
}

// Feed returns complete data fields, joining multiline data with newlines.
// Oversized events are skipped; decoding resumes at the next blank line.
func (decoder *Decoder) Feed(chunk []byte) [][]byte {
	var events [][]byte
	for _, character := range chunk {
		if decoder.skipLF && character == '\n' {
			decoder.skipLF = false
			continue
		}
		decoder.skipLF = character == '\r'
		if character == '\r' || character == '\n' {
			if data := decoder.endLine(); data != nil {
				events = append(events, data)
			}
			decoder.line = decoder.line[:0]
			continue
		}
		if len(decoder.line) < decoder.limit {
			decoder.line = append(decoder.line, character)
		} else {
			decoder.oversized = true
		}
	}
	return events
}

func (decoder *Decoder) endLine() []byte {
	// The tagged SSE line planner does not strip a BOM from a wire field
	// name. Only the assembled first data value has its BOM removed by
	// Mojo runtime_sse_payload_join (including repeated prefixes).
	line := decoder.line
	if len(line) == 0 {
		data := decoder.data
		decoder.data = nil
		oversized := decoder.oversized
		decoder.oversized = false
		if !oversized && len(data) > 0 {
			return data[:len(data)-1]
		}
		return nil
	}
	if decoder.oversized || line[0] == ':' {
		return nil
	}
	field, value, _ := bytes.Cut(line, []byte(":"))
	if !bytes.Equal(field, []byte("data")) {
		return nil
	}
	value = bytes.TrimPrefix(value, []byte(" "))
	if len(decoder.data)+len(value)+1 > decoder.limit {
		decoder.oversized = true
		decoder.data = nil
		return nil
	}
	if len(decoder.data) == 0 {
		// Source runtime_sse_payload_join consumes every leading BOM from
		// the first data value, even when the physical field starts after
		// ordinary comments. Subsequent data lines keep their literal BOMs.
		for bytes.HasPrefix(value, []byte("\xef\xbb\xbf")) {
			value = value[3:]
		}
	}
	decoder.data = append(decoder.data, value...)
	decoder.data = append(decoder.data, '\n')
	return nil
}

// Finish flushes a final unterminated line/event without exceeding decoder bounds.
func (decoder *Decoder) Finish() [][]byte {
	var events [][]byte
	if len(decoder.line) > 0 {
		if data := decoder.endLine(); data != nil {
			events = append(events, data)
		}
		decoder.line = decoder.line[:0]
	}
	if data := decoder.endLine(); data != nil {
		events = append(events, data)
	}
	decoder.line = decoder.line[:0]
	decoder.skipLF = false
	return events
}
