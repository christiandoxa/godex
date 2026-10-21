package gemini

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
)

type geminiTerminalReadProbe04384 struct {
	payload []byte
	reads   atomic.Int32
	closed  chan struct{}
}

func (source *geminiTerminalReadProbe04384) Read(data []byte) (int, error) {
	if source.reads.Add(1) == 1 {
		return copy(data, source.payload), nil
	}
	return 0, io.EOF
}

func (source *geminiTerminalReadProbe04384) Close() error {
	close(source.closed)
	return nil
}

func TestProdex04384GeminiSSEDoneStopsReadingUpstream(t *testing.T) {
	for _, native := range []bool{false, true} {
		chunk := `{"choices":[{"delta":{"content":"terminal-control"}}]}`
		if native {
			chunk = `{"candidates":[{"content":{"parts":[{"text":"terminal-control"}]}}]}`
		}
		source := &geminiTerminalReadProbe04384{payload: []byte("data: " + chunk + "\n\ndata: [DONE]\n\n"), closed: make(chan struct{})}
		upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: source}
		var body io.ReadCloser
		if native {
			body = translateGeminiNativeStream(upstream, nil).Body
		} else {
			body = translateGeminiChatCompatibleStream043711(upstream).Body
		}
		if _, err := io.ReadAll(body); err != nil {
			t.Fatal(err)
		}
		body.Close()
		<-source.closed
		if reads := source.reads.Load(); reads != 1 {
			t.Errorf("terminal stream kept reading upstream: native=%v reads=%d", native, reads)
		}
	}
}
