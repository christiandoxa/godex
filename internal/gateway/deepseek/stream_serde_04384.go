package deepseek

import "github.com/christiandoxa/godex/internal/helper/jsonutil"

// Tagged DeepSeek 0.438.4 stream.rs calls serde_json::from_str::<Value>
// on every complete SSE data object before emitting deltas or tool events.
// Do not let Go's decoder fabricate output from otherwise invalid JSON.
func deepSeekValidStreamValue04384(data []byte) bool {
	return jsonutil.CheckSerdeBoundary(string(data)) == nil && jsonutil.CheckSerdeNumberTokens(data)
}

func firstDeepSeekStreamChoice(root map[string]any) map[string]any {
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	return choice
}
