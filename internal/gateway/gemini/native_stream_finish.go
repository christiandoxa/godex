package gemini

import "io"

func (state *geminiStreamState) finishChunk(writer io.Writer, root, candidate map[string]any) error {
	status := make(map[string]any)
	geminiApplyNativeStatus(status, root, candidate, true)
	if status["status"] == "failed" {
		failure := status["error"].(map[string]any)
		return state.failed(writer, failure["code"].(string), failure["message"].(string))
	}
	if err := state.completeTools(writer); err != nil {
		return err
	}
	if status["status"] != "incomplete" {
		return nil
	}
	state.completed = true
	return writeGeminiSSEEvent(writer, "response.incomplete", map[string]any{
		"type": "response.incomplete", "sequence_number": state.nextSequence(),
		"response": map[string]any{
			"id": state.responseID, "status": "incomplete", "incomplete_details": geminiMaxTokensIncompleteDetails(),
		},
	})
}
