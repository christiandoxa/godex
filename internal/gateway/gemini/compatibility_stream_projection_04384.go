package gemini

import (
	"context"
	"errors"
	"io"
)

type geminiChatProjectionPending04384 struct{}

func (geminiChatProjectionPending04384) Error() string { return "unsupported Gemini stream projection" }

type geminiChatStreamSource04384 struct {
	source io.Closer
	cancel context.CancelFunc
}

// PipeBody owns the single upstream close and the translated reader's cancellation.
func (source *geminiChatStreamSource04384) Close() error {
	source.cancel()
	return source.source.Close()
}

func pendingGeminiChatProjection04384(chunk map[string]any) bool {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return false
	}
	choice, object := choices[0].(map[string]any)
	if !object {
		return true
	}
	delta, _ := choice["delta"].(map[string]any)
	calls, _ := delta["tool_calls"].([]any)
	for _, call := range calls {
		if _, object := call.(map[string]any); !object {
			return true
		}
	}
	return false
}

func (state *geminiChatCompatibleStream043711) handleTranslationFailure(ctx context.Context, writer *io.PipeWriter, cause error) {
	var pending geminiChatProjectionPending04384
	if errors.As(cause, &pending) {
		// The tagged projection leaves this frame pending; retain no work while waiting.
		<-ctx.Done()
		_ = writer.CloseWithError(ctx.Err())
		return
	}
	_ = state.failure(writer, "provider_stream_error", "provider stream failed")
}
