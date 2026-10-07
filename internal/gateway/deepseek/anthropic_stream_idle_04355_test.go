package deepseek

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type delayedAnthropicBody struct {
	mu     sync.Mutex
	chunks [][]byte
	delays []time.Duration
	index  int
	closed chan struct{}
	once   sync.Once
}

func newDelayedAnthropicBody(chunks []string, delays []time.Duration) *delayedAnthropicBody {
	values := make([][]byte, len(chunks))
	for index, chunk := range chunks {
		values[index] = []byte(chunk)
	}
	return &delayedAnthropicBody{chunks: values, delays: delays, closed: make(chan struct{})}
}

func (body *delayedAnthropicBody) Read(buffer []byte) (int, error) {
	body.mu.Lock()
	index := body.index
	if index >= len(body.chunks) {
		body.mu.Unlock()
		return 0, io.EOF
	}
	body.index++
	delay := time.Duration(0)
	if index < len(body.delays) {
		delay = body.delays[index]
	}
	chunk := append([]byte(nil), body.chunks[index]...)
	body.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-body.closed:
			return 0, io.ErrClosedPipe
		}
	}
	return copy(buffer, chunk), nil
}

func (body *delayedAnthropicBody) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

func TestProdex04355DeepSeekStreamIdleAcceptsSubIdleGaps(t *testing.T) {
	stream := runNativeIdleFixture(t,
		[]string{
			`event: message_start` + "\ndata: " + `{"type":"message_start","message":{"id":"msg_idle"}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}` + "\n\n" +
				`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ready"}}` + "\n\n",
			`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n",
		},
		[]time.Duration{0, 50 * time.Millisecond, 50 * time.Millisecond},
		500*time.Millisecond,
	)
	if !strings.Contains(stream, `"delta":"ready"`) || !strings.Contains(stream, "event: response.completed") {
		t.Fatalf("sub-idle stream = %s", stream)
	}
}

func TestProdex04355DeepSeekStreamIdleResetsAfterEachChunk(t *testing.T) {
	stream := runNativeIdleFixture(t,
		[]string{
			`event: message_start` + "\ndata: " + `{"type":"message_start","message":{"id":"msg_reset"}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}` + "\n\n",
			`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"a"}}` + "\n\n",
			`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"b"}}` + "\n\n" +
				`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n",
		},
		[]time.Duration{0, 100 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond},
		250*time.Millisecond,
	)
	if !strings.Contains(stream, `"delta":"a"`) || !strings.Contains(stream, `"delta":"b"`) || !strings.Contains(stream, "event: response.completed") {
		t.Fatalf("reset-idle stream = %s", stream)
	}
}

func TestProdex04355DeepSeekStreamIdleTimeoutFailsWithoutHanging(t *testing.T) {
	started := time.Now()
	stream := runNativeIdleFixture(t,
		[]string{
			`event: message_start` + "\ndata: " + `{"type":"message_start","message":{"id":"msg_timeout"}}` + "\n\n",
			`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n",
		},
		[]time.Duration{0, 250 * time.Millisecond},
		25*time.Millisecond,
	)
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("idle timeout took %s", elapsed)
	}
	if !strings.Contains(stream, "event: response.failed") || !strings.Contains(stream, "provider_stream_error") || strings.Contains(stream, "event: response.completed") {
		t.Fatalf("idle timeout stream = %s", stream)
	}
}

func runNativeIdleFixture(t *testing.T, chunks []string, delays []time.Duration, idle time.Duration) string {
	t.Helper()
	body := newDelayedAnthropicBody(chunks, delays)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return anthropicStreamResponse(request, body), nil
	})}
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: 100 * time.Millisecond, StreamIdleTimeout: idle,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"deepseek-v4-pro","input":"search","web_search_options":{}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
