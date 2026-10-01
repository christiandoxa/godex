package copilot

import (
	"bytes"
	"encoding/json"
	"strings"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

func copilotModelFallbackChain(body []byte) []string {
	object, ok := decodeJSONObject(body)
	if !ok {
		return []string{defaultRuntimeModel}
	}
	model, _ := object["model"].(string)
	return providerentity.ModelFallbackChain("copilot", model)
}

func copilotRequestBodyWithModel(body []byte, model string) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return append([]byte(nil), body...)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return append([]byte(nil), body...)
	}
	object["model"] = strings.TrimSpace(model)
	stripEncryptedContent(object, false)
	rewritten, err := json.Marshal(object)
	if err != nil {
		return append([]byte(nil), body...)
	}
	return rewritten
}
