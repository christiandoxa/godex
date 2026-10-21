package deepseek

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type deepSeekBodyCloseProbe04384 struct {
	packet    io.Reader
	started   chan struct{}
	closed    chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
	closes    atomic.Int32
}

func (source *deepSeekBodyCloseProbe04384) Read(data []byte) (int, error) {
	source.readOnce.Do(func() { close(source.started) })
	if source.packet != nil {
		return source.packet.Read(data)
	}
	<-source.closed
	return 0, io.EOF
}

func (source *deepSeekBodyCloseProbe04384) Close() error {
	source.closes.Add(1)
	source.closeOnce.Do(func() { close(source.closed) })
	return nil
}

func TestProdex04384DeepSeekStreamBodyCloseCancelsBlockedUpstream(t *testing.T) {
	for _, native := range []bool{false, true} {
		source := &deepSeekBodyCloseProbe04384{started: make(chan struct{}), closed: make(chan struct{})}
		var translated io.ReadCloser
		if native {
			translated = deepSeekAnthropicSSE(source, nil)
		} else {
			translated = deepSeekChatSSE(source)
		}
		<-source.started
		if err := translated.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-source.closed:
		default:
			t.Errorf("client Close left upstream Read blocked: native=%v", native)
		}
		source.Close() // Release the fail-before reader without a timer or leaked goroutine.
	}
}

func TestProdex04384DeepSeekStreamSourceClosesOnceAcrossCompletionAndClientClose(t *testing.T) {
	for _, native := range []bool{false, true} {
		wire := "data: {\"choices\":[{\"delta\":{\"content\":\"close-control\"}}]}\n\ndata: [DONE]\n\n"
		if native {
			wire = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-close\",\"model\":\"deepseek-v4-pro\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
		}
		source := &deepSeekBodyCloseProbe04384{packet: strings.NewReader(wire), started: make(chan struct{}), closed: make(chan struct{})}
		var translated io.ReadCloser
		if native {
			translated = deepSeekAnthropicSSE(source, nil)
		} else {
			translated = deepSeekChatSSE(source)
		}
		body, err := io.ReadAll(translated)
		if err != nil || !strings.Contains(string(body), "event: response.completed") {
			t.Fatalf("normal completion failed: native=%v error=%v body=%s", native, err, body)
		}
		<-source.closed
		translated.Close()
		translated.Close()
		if count := source.closes.Load(); count != 1 {
			t.Errorf("upstream closed repeatedly after completion: native=%v closes=%d", native, count)
		}
	}
}
