package deepseek

import (
	"errors"
	"io"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

func deepSeekAnthropicSSE(body io.ReadCloser, requestMetadata map[string]any) io.ReadCloser {
	return deepSeekAnthropicSSEWithRequestID(body, requestMetadata, 0)
}

func deepSeekAnthropicSSEWithRequestID(body io.ReadCloser, requestMetadata map[string]any, requestID uint64) io.ReadCloser {
	return deepSeekAnthropicSSEWithConversation(body, requestMetadata, requestID, chatcompat.ConversationStore{}, nil)
}

func deepSeekAnthropicSSEWithConversation(
	body io.ReadCloser,
	requestMetadata map[string]any,
	requestID uint64,
	conversations chatcompat.ConversationStore,
	conversationMessages []any,
) io.ReadCloser {
	reader, writer := io.Pipe()
	stream := sse.NewPipeBody(reader, body)
	go func() {
		defer stream.CloseSource()
		pumpDeepSeekAnthropicSSE(
			body, writer, requestMetadata, requestID, conversations, conversationMessages,
		)
	}()
	return stream
}

func pumpDeepSeekAnthropicSSE(
	body io.ReadCloser,
	writer *io.PipeWriter,
	requestMetadata map[string]any,
	requestID uint64,
	conversations chatcompat.ConversationStore,
	conversationMessages []any,
) {
	decoder := sse.NewDecoder(nativeMessagesMaxBytes)
	state := anthropicStreamState{
		requestID: requestID, requestMetadata: requestMetadata,
		conversations: conversations, conversationMessages: chatcompat.CloneMessages(conversationMessages),
	}
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			for _, event := range decoder.Feed(buffer[:read]) {
				translated, supported, translateErr := state.translate(event, time.Now())
				if translateErr != nil {
					writeAnthropicStreamFailure(writer, &state)
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

func writeAnthropicStreamFailure(writer *io.PipeWriter, state *anthropicStreamState) {
	failed, supported, _ := state.failed("provider_stream_error", "Anthropic stream failed", time.Now())
	if supported {
		if _, err := writer.Write(failed); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
	}
	_ = writer.Close()
}
