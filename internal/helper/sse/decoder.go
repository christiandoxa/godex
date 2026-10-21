// Package sse decodes bounded event data without changing stream framing.
package sse

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"
)

type Decoder struct {
	limit      int
	line       []byte
	data       []byte
	oversized  bool
	strict     bool
	eventBytes int
	err        error
}

// NewStrictDecoder fails on oversized wire lines/events and invalid UTF-8.
// Wire line bounds include CR/LF; event bounds include one byte per data line.
func NewStrictDecoder(limit int) *Decoder {
	return &Decoder{limit: max(1, limit), strict: true}
}

// Err reports a terminal framing failure from a strict decoder.
func (decoder *Decoder) Err() error { return decoder.err }

func NewDecoder(limit int) *Decoder {
	return &Decoder{limit: max(1, limit)}
}

// Feed returns complete data fields, joining multiline data with newlines.
// NewDecoder skips oversized events; strict decoders stop and report Err.
func (decoder *Decoder) Feed(chunk []byte) [][]byte {
	var events [][]byte
	for _, character := range chunk {
		if decoder.err != nil {
			break
		}
		if decoder.strict && len(decoder.line) >= decoder.limit {
			decoder.err = fmt.Errorf("provider SSE line exceeded safe size limit (%d)", decoder.limit)
			break
		}
		// Tagged runtime_sse_chunk_line_offsets only ends lines at LF;
		// a standalone CR stays inside a data field until LF or EOF.
		if character == '\n' {
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
	if decoder.strict && !utf8.Valid(decoder.line) {
		decoder.err = errors.New("stream did not contain valid UTF-8")
		return nil
	}
	// The tagged SSE line planner does not strip a BOM from a wire field
	// name. Only the assembled first data value has its BOM removed by
	// Mojo runtime_sse_payload_join (including repeated prefixes).
	line := bytes.TrimRight(decoder.line, "\r")
	if len(line) == 0 {
		data := decoder.data
		decoder.data = nil
		decoder.eventBytes = 0
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
	if decoder.strict && len(value)+1 > decoder.limit-decoder.eventBytes {
		decoder.err = fmt.Errorf("provider SSE event exceeded safe size limit (%d)", decoder.limit)
		decoder.data = nil
		return nil
	}
	if !decoder.strict && len(decoder.data)+len(value)+1 > decoder.limit {
		decoder.oversized = true
		decoder.data = nil
		return nil
	}
	decoder.eventBytes += len(value) + 1
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
	if decoder.err != nil {
		return nil
	}
	if len(decoder.line) > 0 {
		if data := decoder.endLine(); data != nil {
			events = append(events, data)
		}
		decoder.line = decoder.line[:0]
	}
	if decoder.err != nil {
		return events
	}
	if data := decoder.endLine(); data != nil {
		events = append(events, data)
	}
	decoder.line = decoder.line[:0]
	return events
}
