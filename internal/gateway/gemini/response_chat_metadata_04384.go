package gemini

import (
	"bytes"
	"io"
	"net/http"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged metadata merges immediate subobject fields; nested objects replace whole values.
func mergeGeminiChatMetadata04384(target, source map[string]any) {
	for key, value := range source {
		incoming, object := value.(map[string]any)
		current, exists := target[key]
		if !object || !exists {
			target[key] = value
			continue
		}
		if existing, ok := current.(map[string]any); ok {
			for nested, value := range incoming {
				existing[nested] = value
			}
		}
	}
}

func geminiChatFallbackResponse04384(status int, requestMetadata map[string]any) (*proxymodel.Response, error) {
	metadata := make(map[string]any, len(requestMetadata))
	mergeGeminiChatMetadata04384(metadata, requestMetadata)
	body, err := jsonutil.MarshalSerdeValue04384(map[string]any{
		"id": "gemini_resp_prodex", "model": "gemini-2.5-pro", "object": "response",
		"output": []any{}, "metadata": metadata, "usage": map[string]any{}, "status": "failed",
		"error": map[string]any{
			"code": "gemini_empty_response", "message": "Gemini returned no visible response content.",
		},
	})
	if err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body: io.NopCloser(bytes.NewReader(body)), Trailer: make(http.Header),
	}, nil
}
