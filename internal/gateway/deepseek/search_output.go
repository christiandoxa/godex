package deepseek

import (
	"encoding/json"
	"fmt"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func deepSeekBufferedSearchArguments(call map[string]any, arguments string) (json.RawMessage, error) {
	callID, ok := call["id"].(string)
	if !ok {
		callID = "call_0"
	}
	id, err := jsonutil.MarshalSerdeValue04384(callID)
	if err != nil {
		return nil, err
	}
	// Mojo embeds search arguments verbatim; serde then parses this whole envelope.
	output := `{"type":"tool_search_call","call_id":` + string(id) + `,"execution":"client","arguments":` + arguments + `}`
	value, err := deepSeekToolArgumentValue(output)
	if err != nil {
		if reason, offset := deepSeekBufferedSerdeFailure(output); reason != "" {
			err = deepSeekSerdeDiagnostic(output, reason, offset)
		}
		// shortcut: remaining output lexical diagnostics need their own pinned serde regressions.
		return nil, fmt.Errorf("DeepSeek Mojo returned invalid response tool-call JSON: %w", err)
	}
	item := value.(map[string]any)
	return jsonutil.MarshalSerdeValue04384(item["arguments"])
}
