package gemini

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type geminiBodyCloseProbe04384 struct {
	packet    io.Reader
	started   chan struct{}
	closed    chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
	closes    atomic.Int32
}

func (source *geminiBodyCloseProbe04384) Read(data []byte) (int, error) {
	source.readOnce.Do(func() { close(source.started) })
	if source.packet != nil {
		return source.packet.Read(data)
	}
	<-source.closed
	return 0, io.EOF
}

func (source *geminiBodyCloseProbe04384) Close() error {
	source.closes.Add(1)
	source.closeOnce.Do(func() { close(source.closed) })
	return nil
}

func TestProdex04384GeminiStreamBodyCloseCancelsBlockedUpstream(t *testing.T) {
	for _, native := range []bool{false, true} {
		source := &geminiBodyCloseProbe04384{started: make(chan struct{}), closed: make(chan struct{})}
		upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: source}
		var body io.ReadCloser
		if native {
			body = translateGeminiNativeStream(upstream, nil).Body
		} else {
			body = translateGeminiChatCompatibleStream043711(upstream, nil).Body
		}
		<-source.started
		if err := body.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-source.closed:
		default:
			t.Errorf("client Close left upstream Read blocked: native=%v", native)
			source.Close()
		}
	}
}

func TestProdex04384GeminiSourceClosesOnceAfterCompletionAndRepeatedClientClose(t *testing.T) {
	for _, native := range []bool{false, true} {
		packet := `{"choices":[{"delta":{"content":"close-control"}}]}`
		if native {
			packet = `{"candidates":[{"content":{"parts":[{"text":"close-control"}]}}]}`
		}
		source := &geminiBodyCloseProbe04384{packet: strings.NewReader("data: " + packet + "\n\ndata: [DONE]\n\n"), started: make(chan struct{}), closed: make(chan struct{})}
		upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: source}
		var body io.ReadCloser
		if native {
			body = translateGeminiNativeStream(upstream, nil).Body
		} else {
			body = translateGeminiChatCompatibleStream043711(upstream, nil).Body
		}
		output, err := io.ReadAll(body)
		if err != nil || !strings.Contains(string(output), "event: response.completed") {
			t.Fatalf("completion failed: native=%v error=%v", native, err)
		}
		<-source.closed
		body.Close()
		body.Close()
		if closes := source.closes.Load(); closes != 1 {
			t.Errorf("source closed repeatedly: native=%v closes=%d", native, closes)
		}
	}
}
