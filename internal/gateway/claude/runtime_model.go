package claude

import (
	"bytes"
	"encoding/json"
	"strings"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
)

func anthropicModelFallbackChain(body []byte) []string {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return []string{"claude-sonnet-4-6"}
	}
	model, _ := object["model"].(string)
	return providerentity.ModelFallbackChain("anthropic", model)
}

func anthropicRequestBodyWithModel(body []byte, model string) []byte {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return append([]byte(nil), body...)
	}
	object["model"] = strings.TrimSpace(model)
	rewritten, err := json.Marshal(object)
	if err != nil {
		return append([]byte(nil), body...)
	}
	return rewritten
}
