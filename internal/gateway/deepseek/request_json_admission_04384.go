package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

type nonobjectResponsesInputError04384 struct{}

func (*nonobjectResponsesInputError04384) Error() string {
	return "DeepSeek Responses request body must be a JSON object"
}

func parseResponsesObjectWithoutValidation(body []byte) (map[string]any, error) {
	object, err := decodeDeepSeekResponsesObject(body)
	if err != nil {
		var nonobject *nonobjectResponsesInputError04384
		if errors.As(err, &nonobject) {
			return nil, err
		}
		return nil, &invalidResponsesInputError{err}
	}
	return object, nil
}

func decodeDeepSeekResponsesObject(body []byte) (map[string]any, error) {
	if len(body) > requestMaxBytes {
		return nil, fmt.Errorf("DeepSeek Responses request exceeds %d bytes", requestMaxBytes)
	}
	// Tagged 0.438.4 DeepSeek request_transform.rs parses the ENTIRE raw
	// value with serde_json before using input, model, tools or history.
	// Invalid unused Unicode, excessive nesting and near-f64-maximum
	// numbers must not be silently discarded and then replayed as valid.
	if jsonutil.CheckSerdeBoundary(string(body)) != nil || !jsonutil.CheckSerdeNumberTokens(body) {
		return nil, errors.New("failed to parse DeepSeek Responses request JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse DeepSeek Responses request JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("DeepSeek Responses request body must contain exactly one JSON value")
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, &nonobjectResponsesInputError04384{}
	}
	return object, nil
}
