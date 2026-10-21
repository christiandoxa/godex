package deepseek

import (
	"errors"
	"io"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func deepSeekChatSSE(body io.ReadCloser) io.ReadCloser {
	return deepSeekChatSSEWithConversation(body, 0, nil, nil, deepSeekConversationStore{})
}

func deepSeekChatSSEWithConversation(
	body io.ReadCloser,
	requestID uint64,
	conversationMessages []any,
	responseMetadata map[string]any,
	conversations deepSeekConversationStore,
) io.ReadCloser {
	reader, writer := io.Pipe()
	state := newDeepSeekChatStreamState(requestID, conversationMessages, responseMetadata, conversations)
	stream := sse.NewPipeBody(reader, body)
	go func() {
		defer stream.CloseSource()
		pumpDeepSeekChatSSE(body, writer, state)
	}()
	return stream
}

func pumpDeepSeekChatSSE(body io.ReadCloser, writer *io.PipeWriter, state *deepSeekChatStreamState) {
	decoder := sse.NewStrictDecoder(4 << 20)
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			for _, event := range decoder.Feed(buffer[:read]) {
				translated, supported, translateErr := state.observe(event)
				if translateErr != nil {
					failed, failedSupported, failedErr := state.failed("provider_stream_error", "DeepSeek stream failed")
					if failedErr == nil && failedSupported {
						_, failedErr = writer.Write(failed)
					}
					if failedErr != nil {
						_ = writer.CloseWithError(failedErr)
					} else {
						_ = writer.Close()
					}
					return
				}
				if supported {
					if _, writeErr := writer.Write(translated); writeErr != nil {
						_ = writer.CloseWithError(writeErr)
						return
					}
				}
				if state.completed {
					_ = writer.Close()
					return
				}
			}
		}
		if decoder.Err() != nil {
			failed, _, _ := state.failed("provider_stream_error", decoder.Err().Error())
			_, writeErr := writer.Write(failed)
			_ = writer.CloseWithError(writeErr)
			return
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				for _, data := range decoder.Finish() {
					translated, supported, translateErr := state.observe(data)
					if translateErr != nil {
						failed, failedSupported, failedErr := state.failed("provider_stream_error", "DeepSeek stream failed")
						if failedErr == nil && failedSupported {
							_, failedErr = writer.Write(failed)
						}
						if failedErr != nil {
							_ = writer.CloseWithError(failedErr)
						} else {
							_ = writer.Close()
						}
						return
					}
					if supported {
						if _, writeErr := writer.Write(translated); writeErr != nil {
							_ = writer.CloseWithError(writeErr)
							return
						}
					}
				}
			}
			if !state.completed {
				message := "DeepSeek stream failed"
				if decoder.Err() != nil {
					message = decoder.Err().Error()
				} else if errors.Is(err, io.EOF) {
					message = "unexpected end of stream"
					if state.json.Pending() {
						message = "incomplete JSON segment"
					}
				}
				failed, supported, failErr := state.failed("provider_stream_error", message)
				if failErr != nil {
					_ = writer.CloseWithError(failErr)
					return
				}
				if supported {
					if _, writeErr := writer.Write(failed); writeErr != nil {
						_ = writer.CloseWithError(writeErr)
						return
					}
				}
			}
			_ = writer.Close()
			return
		}
	}
}

func translateDeepSeekSSEData(data []byte) ([]byte, bool, error) {
	state := newDeepSeekChatStreamState(0, nil, nil, deepSeekConversationStore{})
	state.createdAt = 0
	return state.observe(data)
}
