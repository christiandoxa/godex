package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type headerFlushReadProbe04384 struct {
	recorder *httptest.ResponseRecorder
	flushed  bool
}

func (body *headerFlushReadProbe04384) Read([]byte) (int, error) {
	body.flushed = body.recorder.Flushed
	return 0, io.EOF
}

func TestProdex04384StreamHeadersFlushBeforeWaitingForBody(t *testing.T) {
	for _, fixture := range []struct {
		name, contentType string
		flushed           bool
	}{
		{"stream", "text/event-stream; charset=utf-8", true},
		{"buffered", "application/json", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			body := &headerFlushReadProbe04384{recorder: writer}
			response := &proxymodel.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {fixture.contentType}}, Body: io.NopCloser(body)}
			(&Proxy{}).forwardResponse(context.Background(), writer, response, nil, "", &requestLifecycle{})
			if body.flushed != fixture.flushed || writer.Code != http.StatusOK || writer.Body.Len() != 0 || writer.Header().Get("Content-Type") != fixture.contentType {
				t.Fatalf("headers changed before body read: flushed=%v status=%d", body.flushed, writer.Code)
			}
		})
	}
}
