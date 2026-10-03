package deepseek

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func deepSeekAnthropicSSE(body io.ReadCloser, requestMetadata map[string]any) io.ReadCloser {
	return deepSeekAnthropicSSEWithRequestID(body, requestMetadata, 0)
}

func deepSeekAnthropicSSEWithRequestID(body io.ReadCloser, requestMetadata map[string]any, requestID uint64) io.ReadCloser {
	reader, writer := io.Pipe()
	go pumpDeepSeekAnthropicSSE(body, writer, requestMetadata, requestID)
	return &anthropicPipeBody{reader: reader, source: body}
}

type anthropicPipeBody struct {
	reader *io.PipeReader
	source io.Closer
	once   sync.Once
	err    error
}

func (body *anthropicPipeBody) Read(buffer []byte) (int, error) { return body.reader.Read(buffer) }

func (body *anthropicPipeBody) Close() error {
	body.once.Do(func() {
		_ = body.reader.Close()
		body.err = body.source.Close()
	})
	return body.err
}

func pumpDeepSeekAnthropicSSE(body io.ReadCloser, writer *io.PipeWriter, requestMetadata map[string]any, requestID uint64) {
	defer body.Close()
	decoder := sse.NewDecoder(nativeMessagesMaxBytes)
	state := anthropicStreamState{requestID: requestID, requestMetadata: requestMetadata}
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			for _, event := range decoder.Feed(buffer[:read]) {
				translated, supported, translateErr := state.translate(event, time.Now())
				if translateErr != nil {
					_ = writer.CloseWithError(translateErr)
					return
				}
				if supported {
					if _, writeErr := writer.Write(translated); writeErr != nil {
						_ = writer.CloseWithError(writeErr)
						return
					}
					if state.completed {
						_ = writer.Close()
						return
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !state.completed {
					translated, supported, _ := state.failed(
						"provider_stream_error", "unexpected end of Anthropic stream", time.Now(),
					)
					if supported {
						if _, writeErr := writer.Write(translated); writeErr != nil {
							_ = writer.CloseWithError(writeErr)
							return
						}
					}
				}
				_ = writer.Close()
				return
			}
			translated, supported, _ := state.failed("provider_stream_error", "Anthropic stream failed", time.Now())
			if supported {
				if _, writeErr := writer.Write(translated); writeErr != nil {
					_ = writer.CloseWithError(writeErr)
					return
				}
			}
			_ = writer.Close()
			return
		}
	}
}
