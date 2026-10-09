package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (transport *RuntimeTransport) executeResponses(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	translated, err := translateResponsesRequest(input.Body, requestModel(input.Body))
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	response, err := transport.sendResponses(ctx, input, translated)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return translateResponse(response, translated.metadata)
	}
	return proxyResponse(response), nil
}

func translateResponse(response *http.Response, requestMetadata map[string]any) (*proxymodel.Response, error) {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return translateGeminiNativeStream(response, requestMetadata), nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Gemini translated response")
	}
	if len(body) > bodyMaxBytes {
		return nil, errors.New("Gemini translated response exceeded the safe read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var native map[string]any
	if err := decoder.Decode(&native); err != nil {
		return nil, errors.New("failed to parse Gemini response JSON")
	}
	native = normalizedGeminiResponse(native)
	translated, err := json.Marshal(geminiNativeResponsesValue(native, requestMetadata, time.Now().Unix()))
	if err != nil {
		return nil, errors.New("failed to serialize Gemini Responses JSON")
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "application/json"),
		Body:       io.NopCloser(bytes.NewReader(translated)),
		Trailer:    response.Trailer.Clone(),
	}, nil
}

func mergeResponseMetadata(body []byte, requestMetadata map[string]any) ([]byte, error) {
	if len(requestMetadata) == 0 {
		return body, nil
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.New("failed to parse translated Gemini Responses JSON")
	}
	metadata, _ := response["metadata"].(map[string]any)
	merged := make(map[string]any, len(requestMetadata)+len(metadata))
	for key, value := range requestMetadata {
		merged[key] = value
	}
	mergeMetadataFields(merged, metadata)
	response["metadata"] = merged
	translated, err := json.Marshal(response)
	if err != nil {
		return nil, errors.New("failed to serialize translated Gemini Responses JSON")
	}
	return translated, nil
}

func mergeMetadataFields(target, source map[string]any) {
	for key, value := range source {
		current, exists := target[key]
		incoming, isObject := value.(map[string]any)
		if existing, ok := current.(map[string]any); exists && ok && isObject {
			mergeMetadataFields(existing, incoming)
		} else if !exists {
			target[key] = value
		}
	}
}

func translatedHeaders(source http.Header, contentType string) http.Header {
	header := source.Clone()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Set("Content-Type", contentType)
	return header
}

func requestModel(body []byte) string {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	model, _ := object["model"].(string)
	return strings.TrimSpace(model)
}
