package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func translateResponseWithMetadata(response *http.Response, requestMetadata map[string]any) (*proxymodel.Response, error) {
	return translateResponseWithConversation(response, requestMetadata, deepSeekConversationStore{}, nil, 0)
}

func translateResponseWithConversation(
	response *http.Response,
	requestMetadata map[string]any,
	conversations deepSeekConversationStore,
	conversationMessages []any,
	requestID uint64,
) (*proxymodel.Response, error) {
	contentType := strings.ToLower(response.Header.Get(contentTypeHeader))
	if strings.Contains(contentType, "text/event-stream") {
		header := deepSeekSSEHeaders(response.Header)
		return &proxymodel.Response{
			StatusCode: response.StatusCode, Header: header,
			Body: deepSeekChatSSEWithConversation(response.Body, requestID, conversationMessages, requestMetadata, conversations), Trailer: response.Trailer,
			// The tagged Prodex DeepSeek bridge has selected its translated
			// SSE writer. Embedded provider events (including rate limits)
			// must reach the client, not initiate another credential turn.
			FirstEventCommitted: true,
		}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, translatedResponseBodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read DeepSeek translated response")
	}
	if len(body) > translatedResponseBodyMaxBytes {
		return nil, errors.New("DeepSeek translated response exceeded the safe read limit")
	}
	translated, err := deepSeekChatResponse(body, time.Now())
	if err != nil {
		return nil, err
	}
	result, err := translatedDeepSeekResponse(response, translated, requestMetadata)
	if err == nil {
		deepSeekStoreBufferedConversation(conversations, conversationMessages, body, translated)
	}
	return result, err
}

func translatedHeaders(source http.Header, contentType string) http.Header {
	header := source.Clone()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Set(contentTypeHeader, contentType)
	return header
}

func proxyResponse(response *http.Response) *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}
}

func providerResponseProcessingFailure() *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader("provider response could not be processed")),
	}
}

type bufferedResponse struct {
	StatusCode int
	Header     http.Header
	Trailer    http.Header
	body       []byte
}

func bufferError(response *http.Response) (bufferedResponse, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return bufferedResponse{}, errors.New("failed to read DeepSeek error response before fallback")
	}
	if len(body) > bodyMaxBytes {
		return bufferedResponse{}, errors.New("DeepSeek error response exceeded the safe read limit")
	}
	return bufferedResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), Trailer: response.Trailer.Clone(), body: body}, nil
}

func (response bufferedResponse) proxyResponse() *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: io.NopCloser(bytes.NewReader(response.body)), Trailer: response.Trailer}
}

func requestModel(body []byte) string {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	model, _ := object["model"].(string)
	return strings.TrimSpace(model)
}

func validateRuntimeURL(value string) (*url.URL, error) {
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return nil, errors.New("DeepSeek runtime API URL must not contain whitespace")
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("DeepSeek runtime API URL must be an http(s) URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("DeepSeek runtime API URL must use http or https")
	}
	return parsed, nil
}

func cloneClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			copy.Transport = transport.Clone()
		}
	}
	if transport, ok := copy.Transport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DisableCompression = true
		if transport.ResponseHeaderTimeout == 0 {
			transport.ResponseHeaderTimeout = 30 * time.Second
		}
		if transport.IdleConnTimeout == 0 {
			transport.IdleConnTimeout = 90 * time.Second
		}
		copy.Transport = transport
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func (transport *RuntimeTransport) Close() { transport.client.CloseIdleConnections() }

func deepSeekResponseOptions() chatcompat.ResponseOptions {
	return chatcompat.ResponseOptions{
		ProviderKey:        deepSeekProviderKey,
		AdapterLabel:       "DeepSeek",
		DefaultModel:       "deepseek-chat",
		FallbackResponseID: deepSeekResponseFallbackID,
	}
}

func translatedDeepSeekResponse(
	response *http.Response,
	translated []byte,
	requestMetadata map[string]any,
) (*proxymodel.Response, error) {
	return translatedDeepSeekResponseWithContentType(response, translated, requestMetadata, "application/json; charset=utf-8")
}

func translatedDeepSeekResponseWithContentType(
	response *http.Response,
	translated []byte,
	requestMetadata map[string]any,
	contentType string,
) (*proxymodel.Response, error) {
	if len(requestMetadata) > 0 {
		var value map[string]any
		if err := json.Unmarshal(translated, &value); err != nil {
			return nil, errors.New("failed to parse translated DeepSeek Responses JSON")
		}
		mergeResponseMetadata(value, requestMetadata)
		content, err := json.Marshal(value)
		if err != nil {
			return nil, errors.New("failed to serialize translated DeepSeek Responses JSON")
		}
		translated = content
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedBufferedHeaders(contentType),
		Body:       io.NopCloser(bytes.NewReader(translated)),
		Trailer:    response.Trailer.Clone(),
	}, nil
}

func translatedBufferedHeaders(contentType string) http.Header {
	return http.Header{contentTypeHeader: []string{contentType}}
}
