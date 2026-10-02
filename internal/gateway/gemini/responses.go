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

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"github.com/google/uuid"
)

func (transport *RuntimeTransport) executeResponses(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	translated, err := translateResponsesRequest(input.Body, requestModel(input.Body))
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	response, err := transport.send(ctx, input, current, translated.body)
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
		return &proxymodel.Response{
			StatusCode: response.StatusCode,
			Header:     translatedHeaders(response.Header, "text/event-stream"),
			Body:       chatcompat.ChatSSEWithMetadata(response.Body, "gemini", requestMetadata),
			Trailer:    response.Trailer,
		}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Gemini translated response")
	}
	if len(body) > bodyMaxBytes {
		return nil, errors.New("Gemini translated response exceeded the safe read limit")
	}
	translated, err := chatcompat.ChatResponseWithOptions(body, time.Now(), chatcompat.ResponseOptions{
		ProviderKey: "gemini", AdapterLabel: "Gemini OpenAI-compatible",
		DefaultModel:       "auto",
		FallbackResponseID: func() string { return "resp_gemini_" + uuid.NewString() },
		FallbackCallID:     func(int) string { return "call_gemini_" + uuid.NewString() },
	})
	if err != nil {
		return nil, err
	}
	translated, err = mergeResponseMetadata(translated, requestMetadata)
	if err != nil {
		return nil, err
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
