package gemini

import (
	"io"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func (state *geminiChatCompatibleStream043711) observeMessageMetadata(writer io.Writer, chunk map[string]any) error {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	refusal, _ := delta["refusal"].(string)
	annotations, _ := delta["annotations"].([]any)
	if refusal == "" && len(annotations) == 0 {
		return nil
	}
	size := len(refusal)
	if len(annotations) > 0 {
		wire, err := jsonutil.MarshalSerdeValue04384(annotations)
		if err != nil {
			state.eof = true
			return state.failure(writer, "provider_stream_error", "provider annotations exceeded safe JSON limits")
		}
		size += len(wire)
	}
	if size > geminiStreamMetadataMaxBytes04384-state.messageMetadataBytes {
		state.eof = true
		return state.failure(writer, "provider_stream_error", "provider message metadata exceeded safe size limit (67108864)")
	}
	state.messageMetadataBytes += size
	state.refusal.WriteString(refusal)
	state.annotations = append(state.annotations, annotations...)
	return nil
}
