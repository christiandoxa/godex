package proxy

import (
	"context"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
)

func (proxy *Proxy) forwardResponse(ctx context.Context, writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, accountID string, lifecycle *requestLifecycle) {
	if response == nil {
		return
	}
	stream := strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	if prefix == nil && !stream {
		var err error
		prefix, err = inspectResponse(response.Body, proxy.maxInspect)
		if err != nil {
			http.Error(writer, "upstream response failed before commitment", http.StatusBadGateway)
			return
		}
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
		complete := proxy.forwardStream(ctx, writer, response.Body, prefix, accountID, response.Header)
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

func (proxy *Proxy) forwardStream(ctx context.Context, writer http.ResponseWriter, body io.Reader, prefix []byte, accountID string, headers http.Header) bool {
	forwarder := streamForwarder{
		proxy:     proxy,
		writer:    writer,
		accountID: accountID,
		headers:   headers,
		decoder:   sse.NewDecoder(int(proxy.maxInspect)),
	}
	return forwarder.forward(ctx, body, prefix)
}

type streamForwarder struct {
	proxy     *Proxy
	writer    http.ResponseWriter
	accountID string
	headers   http.Header
	decoder   *sse.Decoder
}

func (forwarder *streamForwarder) forward(ctx context.Context, body io.Reader, prefix []byte) bool {
	if forwarder.write(ctx, prefix) != nil {
		return false
	}
	buffer := make([]byte, 32*1024)
	for {
		read, err := body.Read(buffer)
		if read > 0 && forwarder.write(ctx, buffer[:read]) != nil {
			return false
		}
		if err != nil {
			return err == io.EOF
		}
	}
}

func (forwarder *streamForwarder) write(ctx context.Context, chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	forwarder.remember(ctx, chunk)
	if _, err := forwarder.writer.Write(chunk); err != nil {
		return err
	}
	if flusher, ok := forwarder.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (forwarder *streamForwarder) remember(ctx context.Context, chunk []byte) {
	if forwarder.accountID == "" {
		return
	}
	for _, data := range forwarder.decoder.Feed(chunk) {
		if err := forwarder.proxy.router.Observe(ctx, forwarder.accountID, forwarder.headers, data, false); err != nil {
			panic(http.ErrAbortHandler)
		}
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

func inspectResponse(body io.Reader, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	return data, err
}
