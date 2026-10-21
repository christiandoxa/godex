package deepseek

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func deepSeekToolSearchArguments(arguments string) (json.RawMessage, error) {
	value, err := deepSeekToolArgumentValue(arguments)
	if err != nil {
		return nil, err
	}
	// Keep canonical serde number tokens through the outer Go JSON envelope.
	return jsonutil.MarshalSerdeValue04384(value)
}

func deepSeekToolArgumentValue(arguments string) (any, error) {
	if err := jsonutil.CheckSerdeBoundary(arguments); err != nil {
		return nil, err
	}
	if !json.Valid([]byte(arguments)) || !jsonutil.CheckSerdeNumberTokens([]byte(arguments)) {
		return nil, errors.New("invalid JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}
