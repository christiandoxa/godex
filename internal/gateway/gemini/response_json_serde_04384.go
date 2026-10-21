package gemini

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

// Tagged Prodex 0.438.4 uses serde_json::from_slice::<Value> as the
// authority for a complete buffered Gemini response or a complete SSE
// `data:` value. Go encoding/json otherwise replaces malformed UTF-8 and
// unpaired surrogate escapes, tolerates deeper values, and can admit near-
// maximum floating-point numbers that serde_json 1.0.151 rejects.
// A syntactically valid candidate never authorizes silently ignoring an
// invalid unrelated field in the same upstream response document.
func geminiSerdeJSONValid04384(data []byte) bool {
	return jsonutil.CheckSerdeBoundary(string(data)) == nil && jsonutil.CheckSerdeNumberTokens(data)
}

func geminiSerdeResponseObject04384(data []byte) (map[string]any, bool) {
	if !geminiSerdeJSONValid04384(data) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return object, true
}
