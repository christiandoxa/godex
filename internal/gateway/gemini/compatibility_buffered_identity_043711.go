package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// The pinned Prodex 0.437.11 API-key chat-compatible bridge generates its
// own UUIDv7 Responses identity for an otherwise anonymous native Gemini
// buffered response and omits created_at. Direct Gemini GenerateContent
// transport still retains its separate native response policy.
func normalizeGeminiAPIKeyBufferedIdentity043711(
	response *proxymodel.Response,
) (*proxymodel.Response, error) {
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("read Gemini API-key translated response")
	}
	if len(payload) > bodyMaxBytes {
		return nil, errors.New("Gemini API-key translated response is too large")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, errors.New("invalid Gemini API-key translated response")
	}
	delete(object, "created_at")
	var id string
	if err := json.Unmarshal(object["id"], &id); err == nil &&
		strings.HasPrefix(id, "resp_gemini_") {
		generated, err := uuid.NewV7()
		if err != nil {
			return nil, errors.New("generate tagged Gemini Responses identity")
		}
		object["id"], _ = json.Marshal("resp_deepseek_" + generated.String())
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, errors.New("serialize Gemini API-key response")
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     response.Header,
		Body:       io.NopCloser(bytes.NewReader(normalized)),
		Trailer:    response.Trailer,
	}, nil
}
