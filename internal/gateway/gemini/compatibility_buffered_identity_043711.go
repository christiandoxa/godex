package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// The pinned Prodex 0.437.13 API-key chat-compatible bridge generates a new
// UUIDv7 Responses identity regardless of the upstream native responseId or
// traceId, and omits created_at. Direct native/OAuth Gemini GenerateContent
// retains its separate identity policy.
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
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		// JSON null decodes successfully into a nil map, but cannot own a
		// client-visible Responses ID. Fail closed instead of panicking.
		return nil, errors.New("invalid Gemini API-key translated response")
	}
	delete(object, "created_at")
	// The API-key bridge owns the client-visible Responses identity. Its
	// buffered output must not reuse a provider-supplied response or trace ID.
	generated, err := uuid.NewV7()
	if err != nil {
		return nil, errors.New("generate tagged Gemini Responses identity")
	}
	object["id"], _ = json.Marshal("resp_deepseek_" + generated.String())
	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, errors.New("serialize Gemini API-key response")
	}
	headers := response.Header.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	// Tagged Prodex 0.437.11 writes a normalized UTF-8 JSON content
	// type on the buffered API-key Responses envelope, not the native
	// upstream Content-Type. Preserve other headers and the separate
	// native/OAuth and non-2xx passthrough policies.
	headers.Set("Content-Type", "application/json; charset=utf-8")
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     headers,
		Body:       io.NopCloser(bytes.NewReader(normalized)),
		Trailer:    response.Trailer,
	}, nil
}
