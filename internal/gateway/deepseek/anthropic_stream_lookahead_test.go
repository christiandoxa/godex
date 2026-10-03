package deepseek

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type initialThenGateBody struct {
	initial []byte
	rest    []byte
	waiting chan struct{}
	release chan struct{}
	closed  chan struct{}
	once    sync.Once
	step    int
}

func newInitialThenGateBody(initial, rest string) *initialThenGateBody {
	return &initialThenGateBody{
		initial: []byte(initial), rest: []byte(rest),
		waiting: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{}),
	}
}

func (body *initialThenGateBody) Read(buffer []byte) (int, error) {
	if body.step == 0 {
		body.step++
		return copy(buffer, body.initial), nil
	}
	if body.step == 1 {
		body.step++
		close(body.waiting)
		select {
		case <-body.release:
			return copy(buffer, body.rest), io.EOF
		case <-body.closed:
			return 0, io.ErrClosedPipe
		}
	}
	return 0, io.EOF
}

func (body *initialThenGateBody) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

type failingStreamBody struct{ step int }

func (body *failingStreamBody) Read(buffer []byte) (int, error) {
	if body.step == 0 {
		body.step++
		return copy(buffer, `data: {"type":"message_start"`), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func (*failingStreamBody) Close() error { return nil }

func TestDeepSeekNativeMessagesLookaheadTimeoutReplaysPrefixAndCommits(t *testing.T) {
	continuation := `"msg_timeout","model":"deepseek-v4-pro"}}` + "\n\n" +
		`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}` + "\n\n" +
		`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ready"}}` + "\n\n" +
		`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n"
	body := newInitialThenGateBody(`event: message_start`+"\ndata: "+`{"type":"message_start","message":{"id":`, continuation)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return anthropicStreamResponse(request, body), nil
	})}
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: 25 * time.Millisecond,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	type result struct {
		response *proxymodel.Response
		err      error
	}
	completed := make(chan result, 1)
	go func() {
		response, err := transport.Execute(context.Background(), nativeMessagesTestRequest(), proxymodel.Account{})
		completed <- result{response: response, err: err}
	}()
	select {
	case <-body.waiting:
	case <-time.After(time.Second):
		t.Fatal("lookahead did not start its pending read")
	}
	var got result
	select {
	case got = <-completed:
	case <-time.After(time.Second):
		t.Fatal("lookahead timeout did not commit the stream")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	defer got.response.Body.Close()
	close(body.release)
	stream, err := io.ReadAll(got.response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(string(stream), "response.created") || !strings.Contains(string(stream), `"delta":"ready"`) || !strings.Contains(string(stream), "response.completed") {
		t.Fatalf("calls/stream = %d / %s", calls, stream)
	}
}

func TestDeepSeekNativeMessagesRetriesUpstreamReadFailureBeforeFirstEvent(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return anthropicStreamResponse(request, &failingStreamBody{}), nil
		}
		body := io.NopCloser(strings.NewReader(
			`event: message_start` + "\ndata: " + `{"type":"message_start","message":{"id":"msg_retry"}}` + "\n\n" +
				`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n",
		))
		return anthropicStreamResponse(request, body), nil
	})}
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: time.Second,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), nativeMessagesTestRequest(), proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	stream, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(stream), "response.created") || !strings.Contains(string(stream), "response.completed") {
		t.Fatalf("calls/stream = %d / %s", calls, stream)
	}
}

func TestDeepSeekNativeMessagesReportsReadFailureForCredentialRetry(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return anthropicStreamResponse(request, &failingStreamBody{}), nil
	})}
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: time.Second,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	request := nativeMessagesTestRequest()
	request.Body = []byte(`{"model":"deepseek-v4-flash","input":"search","web_search_options":{}}`)
	response, err := transport.Execute(context.Background(), request, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.FirstEventRetryUsed || response.FirstEventCommitted || response.PrecommitFailure == nil || !response.PrecommitFailure.Transport {
		t.Fatalf("upstream read failure classification = %#v", response)
	}
}

func TestDeepSeekNativeMessagesLookaheadHonorsRequestCancellation(t *testing.T) {
	body := newInitialThenGateBody("", "")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return anthropicStreamResponse(request, body), nil
	})}
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: time.Second,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, err := transport.Execute(ctx, nativeMessagesTestRequest(), proxymodel.Account{})
		completed <- err
	}()
	select {
	case <-body.waiting:
	case <-time.After(time.Second):
		t.Fatal("lookahead did not start its pending read")
	}
	cancel()
	select {
	case err := <-completed:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("cancellation error/calls = %v / %d", err, calls)
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not stop lookahead")
	}
}

func nativeMessagesTestRequest() proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"pro","input":"search","web_search_options":{}}`),
	}
}

func anthropicStreamResponse(request *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{contentTypeHeader: []string{"text/event-stream"}},
		Body: body, Request: request,
	}
}
