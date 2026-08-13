package openai

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const backendAPIPath = "/backend-api"

func (proxy *Proxy) newRequest(request *http.Request, body []byte, token string) (*http.Request, error) {
	target := *proxy.upstream
	target.Path = upstreamPath(proxy.upstream.Path, request.URL.Path)
	target.RawPath = upstreamPath(proxy.upstream.EscapedPath(), request.URL.EscapedPath())
	target.RawQuery = request.URL.RawQuery
	clone := request.Clone(request.Context())
	clone.URL = &target
	clone.Host = target.Host
	clone.RequestURI = ""
	clone.Header = request.Header.Clone()
	removeHopHeaders(clone.Header)
	clone.Header.Set("Authorization", "Bearer "+token)
	if body == nil {
		clone.Body = nil
		clone.GetBody = nil
		clone.ContentLength = 0
	} else {
		clone.Body = io.NopCloser(bytes.NewReader(body))
		clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		clone.ContentLength = int64(len(body))
	}
	return clone, nil
}

func cloneHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			copy.Transport = transport.Clone()
		} else {
			copy.Transport = http.DefaultTransport
		}
	}
	if transport, ok := copy.Transport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DisableCompression = true
		if transport.ResponseHeaderTimeout == 0 {
			transport.ResponseHeaderTimeout = upstreamHeaderWait
		}
		if transport.IdleConnTimeout == 0 {
			transport.IdleConnTimeout = upstreamIdleTime
		}
		copy.Transport = transport
	}
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copy
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("proxy listen address must be host:port: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("proxy listen address must be loopback (127.0.0.1 or ::1)")
	}
	return nil
}

func upstreamPath(basePath, requestPath string) string {
	requestPath = normalizeOpenAIPath(requestPath)
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		return "/" + strings.TrimLeft(requestPath, "/")
	}
	if basePath == backendAPIPath && strings.HasPrefix(requestPath, backendAPIPath) &&
		(requestPath == backendAPIPath || strings.HasPrefix(requestPath, backendAPIPath+"/")) {
		return basePath + strings.TrimPrefix(requestPath, backendAPIPath)
	}
	return basePath + "/" + strings.TrimLeft(requestPath, "/")
}

func normalizeOpenAIPath(requestPath string) string {
	const (
		mountPath    = backendAPIPath + "/prodex"
		upstreamPath = backendAPIPath + "/codex"
	)
	if suffix, ok := strings.CutPrefix(requestPath, mountPath+"/v"); ok {
		if slash := strings.IndexByte(suffix, '/'); slash > 0 && legacyVersionSegment(suffix[:slash]) {
			return upstreamPath + suffix[slash:]
		}
	}
	if suffix, ok := strings.CutPrefix(requestPath, mountPath); ok && (suffix == "" || strings.HasPrefix(suffix, "/")) {
		return upstreamPath + suffix
	}
	return requestPath
}

func legacyVersionSegment(segment string) bool {
	if segment == "" {
		return false
	}
	digit := false
	for _, character := range segment {
		if character >= '0' && character <= '9' {
			digit = true
			continue
		}
		if character != '.' {
			return false
		}
	}
	return digit
}

func readLimited(reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("request body limit exceeded")
	}
	return data, nil
}
