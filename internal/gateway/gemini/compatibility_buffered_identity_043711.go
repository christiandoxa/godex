package gemini

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

// The tagged API-key bridge generates a UUIDv7 only when the original
// upstream id is not a string. Otherwise it keeps the provider transform's
// responseId/traceId identity; the original id is never copied as an alias.
func normalizeGeminiAPIKeyBufferedIdentity043711(
	response *proxymodel.Response, source map[string]any,
) (*proxymodel.Response, error) {
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("read Gemini API-key translated response")
	}
	if len(payload) > bodyMaxBytes {
		return nil, errors.New("Gemini API-key translated response is too large")
	}
	object, valid := geminiSerdeResponseObject04384(payload)
	if !valid || object == nil {
		// JSON null decodes successfully into a nil map, but cannot own a
		// client-visible Responses ID. Fail closed instead of panicking.
		return nil, errors.New("invalid Gemini API-key translated response")
	}
	delete(object, "created_at")
	if _, hasStringID := source["id"].(string); !hasStringID {
		generated, err := uuid.NewV7()
		if err != nil {
			return nil, errors.New("generate tagged Gemini Responses identity")
		}
		object["id"] = "resp_deepseek_" + generated.String()
	}
	normalized, err := jsonutil.MarshalSerdeValue04384(object)
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
