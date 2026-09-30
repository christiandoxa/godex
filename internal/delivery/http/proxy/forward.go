package proxy

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
)

func (proxy *Proxy) forwardResponse(writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, accountID string, lifecycle *requestLifecycle) {
	if response == nil {
		return
	}
	stream := strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	if prefix == nil && !stream {
		prefix, _ = inspectResponse(response.Body, proxy.maxInspect)
	}
	copyResponseHeaders(writer.Header(), response.Header)
	for _, header := range []string{"Content-Type", "Content-Length", "Date"} {
		if !hasHeader(response.Header, header) {
			writer.Header()[header] = nil
		}
	}
	declareResponseTrailers(writer.Header(), response.Header, response.Trailer)
	lifecycle.commit()
	writer.WriteHeader(response.StatusCode)
	if stream {
		complete := proxy.forwardStream(writer, response.Body, prefix, accountID, response.Header)
		copyTrailers(writer.Header(), response.Header, response.Trailer)
		if complete {
			lifecycle.complete()
		} else {
			lifecycle.failAfterCommit()
			panic(http.ErrAbortHandler)
		}
		return
	}
	if len(prefix) > 0 {
		if _, err := writer.Write(prefix); err != nil {
			lifecycle.failAfterCommit()
			panic(http.ErrAbortHandler)
		}
	}
	if !copyResponseBody(writer, response.Body) {
		lifecycle.failAfterCommit()
		panic(http.ErrAbortHandler)
	}
	copyTrailers(writer.Header(), response.Header, response.Trailer)
	lifecycle.complete()
}

func (proxy *Proxy) forwardStream(writer http.ResponseWriter, body io.Reader, prefix []byte, accountID string, headers http.Header) bool {
	forwarder := streamForwarder{
		proxy:     proxy,
		writer:    writer,
		accountID: accountID,
		headers:   headers,
		seen:      make([]byte, 0, len(prefix)),
	}
	return forwarder.forward(body, prefix)
}

type streamForwarder struct {
	proxy     *Proxy
	writer    http.ResponseWriter
	accountID string
	headers   http.Header
	seen      []byte
}

func (forwarder *streamForwarder) forward(body io.Reader, prefix []byte) bool {
	if forwarder.write(prefix) != nil {
		return false
	}
	buffer := make([]byte, 32*1024)
	for {
		read, err := body.Read(buffer)
		if read > 0 && forwarder.write(buffer[:read]) != nil {
			return false
		}
		if err != nil {
			return err == io.EOF
		}
	}
}

func (forwarder *streamForwarder) write(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	forwarder.remember(chunk)
	if _, err := forwarder.writer.Write(chunk); err != nil {
		return err
	}
	if flusher, ok := forwarder.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (forwarder *streamForwarder) remember(chunk []byte) {
	if len(forwarder.seen) >= int(forwarder.proxy.maxInspect) {
		return
	}
	remaining := int(forwarder.proxy.maxInspect) - len(forwarder.seen)
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	forwarder.seen = append(forwarder.seen, chunk...)
	if err := forwarder.proxy.router.Observe(forwarder.accountID, forwarder.headers, forwarder.seen, true); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func copyResponseBody(writer http.ResponseWriter, reader io.Reader) bool {
	buffer := make([]byte, 32*1024)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			if _, writeErr := writer.Write(buffer[:read]); writeErr != nil {
				return false
			}
		}
		if err != nil {
			return err == io.EOF
		}
	}
}

func inspectResponse(body io.Reader, limit int64) ([]byte, bool) {
	if body == nil {
		return nil, true
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	return data, err == nil && int64(len(data)) <= limit
}
