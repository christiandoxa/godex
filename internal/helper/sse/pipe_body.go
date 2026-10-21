package sse

import (
	"io"
	"sync"
)

// PipeBody joins a translated reader's lifetime with its upstream source.
type PipeBody struct {
	reader *io.PipeReader
	source io.Closer
	once   sync.Once
	err    error
}

// NewPipeBody gives client cancellation and producer completion one close owner.
func NewPipeBody(reader *io.PipeReader, source io.Closer) *PipeBody {
	return &PipeBody{reader: reader, source: source}
}

func (body *PipeBody) Read(buffer []byte) (int, error) { return body.reader.Read(buffer) }

// Close cancels pipe reads and interrupts a blocked upstream read.
func (body *PipeBody) Close() error {
	_ = body.reader.Close()
	return body.CloseSource()
}

// CloseSource is deferred by the producer without closing the consumer reader.
func (body *PipeBody) CloseSource() error {
	body.once.Do(func() { body.err = body.source.Close() })
	return body.err
}
