package deepseek

import (
	"io"
	"sync"
)

type streamBody struct {
	reader *io.PipeReader
	source io.Closer
	once   sync.Once
	err    error
}

func (body *streamBody) Read(buffer []byte) (int, error) { return body.reader.Read(buffer) }

func (body *streamBody) Close() error {
	_ = body.reader.Close()
	return body.closeSource()
}

func (body *streamBody) closeSource() error {
	body.once.Do(func() { body.err = body.source.Close() })
	return body.err
}
