package localrewrite

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/httpheader"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const ProviderRuntimeAPIKey = "godex-runtime-provider"

type Transport struct {
	base   *url.URL
	client *http.Client
}

func NewTransport(baseURL string, client *http.Client) (*Transport, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("local rewrite upstream must be an absolute credential-free http(s) URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	return &Transport{base: parsed, client: &copyClient}, nil
}

func (transport *Transport) Close() {
	if transport == nil || transport.client == nil {
		return
	}
	transport.client.CloseIdleConnections()
}

func (transport *Transport) Execute(
	ctx context.Context,
	input proxymodel.Request,
	_ proxymodel.Account,
) (*proxymodel.Response, error) {
	target, err := transport.target(input)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, target.String(), bytes.NewReader(input.Body))
	if err != nil {
		return nil, err
	}
	request.Header = localRewriteHeaders(input.Header)
	request.Header.Set("Authorization", "Bearer "+ProviderRuntimeAPIKey)
	response, err := transport.client.Do(request)
	if err != nil {
		return nil, err
	}
	headers := response.Header.Clone()
	removeResponseHopHeaders(headers)
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     headers,
		Body:       response.Body,
		Trailer:    response.Trailer,
	}, nil
}

func (transport *Transport) target(input proxymodel.Request) (*url.URL, error) {
	pathAndQuery := input.Path
	if input.RawQuery != "" {
		pathAndQuery += "?" + input.RawQuery
	}
	if !RequestTargetValid(pathAndQuery) {
		return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: "local rewrite request target is invalid"}
	}
	suffix := localRewriteSuffix(input.Path)
	if suffix == "" && input.Path != "/backend-api/godex" &&
		input.Path != "/backend-api/prodex" && input.Path != "/v1" {
		return nil, &proxymodel.Error{StatusCode: http.StatusNotFound, Message: "local rewrite route is not supported"}
	}
	target := *transport.base
	basePath := strings.TrimRight(target.Path, "/")
	if suffix != "" {
		if !strings.HasPrefix(suffix, "/") {
			suffix = "/" + suffix
		}
		target.Path = basePath + suffix
	} else {
		target.Path = basePath
	}
	target.RawPath = ""
	target.RawQuery = input.RawQuery
	return &target, nil
}

func localRewriteSuffix(path string) string {
	for _, mount := range []string{"/backend-api/godex", "/backend-api/prodex", "/v1"} {
		if suffix, ok := strings.CutPrefix(path, mount); ok &&
			(suffix == "" || strings.HasPrefix(suffix, "/")) {
			return suffix
		}
	}
	return ""
}

func localRewriteHeaders(source http.Header) http.Header {
	headers := source.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	connection := httpheader.ConnectionTokens(headers)
	for key := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		if httpheader.IsRequestTransport(lower) ||
			connection[http.CanonicalHeaderKey(key)] ||
			strings.HasPrefix(lower, "sec-websocket-") ||
			strings.HasPrefix(lower, "x-godex-internal-") ||
			strings.HasPrefix(lower, "x-prodex-internal-") ||
			lower == "chatgpt-account-id" {
			delete(headers, key)
		}
	}
	return headers
}

func removeResponseHopHeaders(headers http.Header) {
	connection := httpheader.ConnectionTokens(headers)
	for key := range headers {
		if httpheader.IsHop(key) || connection[http.CanonicalHeaderKey(key)] {
			delete(headers, key)
		}
	}
}

func RequestTargetValid(raw string) bool {
	if raw == "" || len(raw) > 8*1024 || !isASCII(raw) {
		return false
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] <= ' ' || raw[index] == 0x7f {
			return false
		}
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "#") {
		return false
	}
	pathEnd := strings.IndexByte(raw, '?')
	if pathEnd < 0 {
		pathEnd = len(raw)
	}
	path := raw[:pathEnd]
	if strings.Contains(path, "\\") || strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] != '%' {
			continue
		}
		if index+2 >= len(raw) {
			return false
		}
		high, ok := hexValue(raw[index+1])
		if !ok {
			return false
		}
		low, ok := hexValue(raw[index+2])
		if !ok {
			return false
		}
		decoded := high<<4 | low
		if index < pathEnd &&
			(decoded >= 0x80 || decoded <= ' ' || decoded == 0x7f ||
				strings.ContainsRune("/\\%?#.", rune(decoded))) {
			return false
		}
		index += 2
	}
	return true
}

func isASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= 0x80 {
			return false
		}
	}
	return true
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
