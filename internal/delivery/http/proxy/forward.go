package proxy

import (
	"context"
	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

func writeTextResponse(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, message)
}

func (proxy *Proxy) forwardResponse(ctx context.Context, writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, accountID string, lifecycle *requestLifecycle, providerKinds ...string) {
	if response == nil {
		return
	}
	stream := strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
	prepared, ok := proxy.prepareResponsePrefix(writer, response, prefix, stream)
	if !ok {
		return
	}
	copyResponseHeaders(writer.Header(), response.Header)
	clearMissingStandardHeaders(writer.Header(), response.Header)
	declareResponseTrailers(writer.Header(), response.Header, response.Trailer)
	lifecycle.commit()
	writer.WriteHeader(response.StatusCode)
	if stream {
		providerKind := ""
		if len(providerKinds) > 0 {
			providerKind = providerKinds[0]
		}
		proxy.finishCommittedStream(ctx, writer, response, prepared, accountID, lifecycle, providerKind)
		return
	}
	proxy.finishCommittedBody(writer, response, prepared, lifecycle)
}

func (proxy *Proxy) prepareResponsePrefix(writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, stream bool) ([]byte, bool) {
	if prefix != nil || stream {
		return prefix, true
	}
	prepared, err := inspectResponse(response.Body, proxy.maxInspect)
	if err != nil {
		writeTextResponse(writer, http.StatusBadGateway, "upstream response failed before commitment")
		return nil, false
	}
	return prepared, true
}

func clearMissingStandardHeaders(destination, source http.Header) {
	// Let net/http add a current Date when upstream did not supply one.
	// A nil Date header suppresses the RFC-required server timestamp and
	// diverges from the Prodex 0.436.1 local proxy response.
	for _, header := range []string{"Content-Type", "Content-Length"} {
		if !hasHeader(source, header) {
			destination[header] = nil
		}
	}
}

func (proxy *Proxy) finishCommittedStream(ctx context.Context, writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, accountID string, lifecycle *requestLifecycle, providerKind string) {
	complete := proxy.forwardStream(ctx, writer, response.Body, prefix, accountID, response.Header, providerKind)
	copyTrailers(writer.Header(), response.Header, response.Trailer)
	if complete {
		lifecycle.complete()
		return
	}
	lifecycle.failAfterCommit()
	panic(http.ErrAbortHandler)
}

func (proxy *Proxy) finishCommittedBody(writer http.ResponseWriter, response *proxymodel.Response, prefix []byte, lifecycle *requestLifecycle) {
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

func (proxy *Proxy) forwardStream(ctx context.Context, writer http.ResponseWriter, body io.Reader, prefix []byte, accountID string, headers http.Header, providerKind string) bool {
	forwarder := streamForwarder{
		proxy:        proxy,
		writer:       writer,
		accountID:    accountID,
		providerKind: providerKind,
		headers:      headers,
		decoder:      sse.NewDecoder(int(proxy.maxInspect)),
	}
	return forwarder.forward(ctx, body, prefix)
}

type streamForwarder struct {
	proxy        *Proxy
	writer       http.ResponseWriter
	accountID    string
	providerKind string
	headers      http.Header
	decoder      *sse.Decoder
}

func (forwarder *streamForwarder) forward(ctx context.Context, body io.Reader, prefix []byte) bool {
	if forwarder.writeChunk(ctx, prefix, false) != nil {
		return false
	}
	buffer := make([]byte, 32*1024)
	for {
		read, err := body.Read(buffer)
		if read > 0 && forwarder.writeChunk(ctx, buffer[:read], true) != nil {
			return false
		}
		if err != nil {
			if err == io.EOF {
				forwarder.rememberEvents(ctx, forwarder.decoder.Finish())
			}
			return err == io.EOF
		}
	}
}

func (forwarder *streamForwarder) writeChunk(ctx context.Context, chunk []byte, observe bool) error {
	if len(chunk) == 0 {
		return nil
	}
	if observe {
		forwarder.remember(ctx, chunk)
	}
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
	forwarder.rememberEvents(ctx, forwarder.decoder.Feed(chunk))
}

func (forwarder *streamForwarder) rememberEvents(ctx context.Context, events [][]byte) {
	if forwarder.accountID == "" {
		return
	}
	for _, data := range events {
		if !utf8.Valid(data) {
			continue
		}
		// The response is already committed. Affinity/usage observation is
		// best-effort and must not reorder or truncate client-visible bytes.
		_ = forwarder.proxy.router.Observe(ctx, forwarder.accountID, forwarder.headers, data, false, forwarder.providerKind)
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
