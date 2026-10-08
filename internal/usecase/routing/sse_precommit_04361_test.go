package routing

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04361SSEWaitsForRetryableSignalAfterMetadata(t *testing.T) {
	reader, writer := io.Pipe()
	body := &notifyingPipeReader{PipeReader: reader, reads: make(chan struct{}, 4)}
	router := &Router{
		now:      time.Now,
		inflight: make(map[string]int), inflightChanged: make(chan struct{}), profileInflightHardLimit: 2,
	}
	request := proxymodel.Request{Path: "/responses", Header: make(http.Header)}
	release, ok := router.tryAcquireProfileInflight("account-a", request, false)
	if !ok {
		t.Fatal("profile admission was rejected")
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       &profileInflightBody{ReadCloser: body, release: release},
	}
	type classified struct {
		outcome responseOutcome
		pending *pendingResponse
		err     error
	}
	result := make(chan classified, 1)
	go func() {
		outcome, pending, err := router.classify(response, "openai")
		result <- classified{outcome: outcome, pending: pending, err: err}
	}()

	<-body.reads
	_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\"}\n\n")
	<-body.reads
	select {
	case got := <-result:
		got.pending.close()
		t.Fatalf("startup metadata committed before retry signal: %#v", got)
	default:
	}
	quota := "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"usage_limit_reached\"}}}\n\n"
	_, _ = io.WriteString(writer, quota)
	got := <-result
	_ = writer.Close()
	defer got.pending.close()
	if got.err != nil || got.outcome.kind != responseRetry || !got.outcome.quota {
		t.Fatalf("delayed retryable stream outcome = %#v, err=%v", got.outcome, got.err)
	}
	if !strings.HasSuffix(string(got.pending.prefix), quota) {
		t.Fatalf("retained prelude = %q", got.pending.prefix)
	}
	router.mu.Lock()
	active := router.inflight["account-a"]
	router.mu.Unlock()
	if active != 0 {
		t.Fatalf("retryable response retained profile admission: %d", active)
	}
}

func TestProdex04361SSEPreviousResponseNotFoundKeepsAdmissionUntilClose(t *testing.T) {
	router := &Router{
		now: time.Now, inflight: make(map[string]int), inflightChanged: make(chan struct{}),
		profileInflightHardLimit: 1,
	}
	request := proxymodel.Request{Path: "/responses", Header: make(http.Header)}
	release, acquired := router.tryAcquireProfileInflight("account-a", request, false)
	if !acquired {
		t.Fatal("profile admission was rejected")
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body: &profileInflightBody{
			ReadCloser: io.NopCloser(strings.NewReader("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"previous_response_not_found\"}}}\n\n")),
			release:    release,
		},
	}
	outcome, pending, err := router.classify(response, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.kind != responsePass || !outcome.previousResponseNotFound {
		t.Fatalf("stale SSE outcome = %#v", outcome)
	}
	router.mu.Lock()
	active := router.inflight["account-a"]
	router.mu.Unlock()
	if active != 2 {
		t.Fatalf("stale SSE released admission before body close: %d", active)
	}
	pending.close()
	router.mu.Lock()
	active = router.inflight["account-a"]
	router.mu.Unlock()
	if active != 0 {
		t.Fatalf("stale SSE admission after body close = %d, want 0", active)
	}
}

func TestProdex04361SSEPrecommitRejectsIncompleteEOFAndByteLimit(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		limit int64
		want  error
	}{
		{name: "metadata eof", body: "data: {\"type\":\"response.created\"}\n\n", limit: 128, want: io.ErrUnexpectedEOF},
		{name: "byte limit", body: ":" + strings.Repeat("x", 65), limit: 64, want: errStreamPrecommitLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := &Router{now: time.Now}
			response := &proxymodel.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(test.body)),
			}
			_, pending, err := router.inspectStreamWithPolicy(response, &pendingResponse{response: response}, "openai", test.limit, time.Second)
			defer pending.close()
			if !errors.Is(err, test.want) {
				t.Fatalf("precommit error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestProdex04361SSEOutputAtByteLimitCommits(t *testing.T) {
	output := "data: {\"type\":\"response.output_text.delta\"}\n\n"
	router := &Router{now: time.Now}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(output)),
	}
	outcome, pending, err := router.inspectStreamWithPolicy(response, &pendingResponse{response: response}, "openai", int64(len(output)), time.Second)
	defer pending.close()
	if err != nil || outcome.kind != responsePass || string(pending.prefix) != output {
		t.Fatalf("boundary output = outcome:%#v prefix:%q err:%v", outcome, pending.prefix, err)
	}
}

func TestProdex04361SSEPrecommitTimeoutClosesBlockedBody(t *testing.T) {
	body := &blockedReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
	router := &Router{now: time.Now}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       body,
	}
	started := time.Now()
	_, pending, err := router.inspectStreamWithPolicy(response, &pendingResponse{response: response}, "openai", 128, 20*time.Millisecond)
	defer pending.close()
	if _, ok := err.(streamPrecommitTimeoutError); !ok {
		t.Fatalf("precommit timeout error = %T %v", err, err)
	}
	select {
	case <-body.started:
	default:
		t.Fatal("source read did not start")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("precommit timeout did not close the upstream body")
	}
	if time.Since(started) > time.Second {
		t.Fatal("precommit deadline did not stop a stalled stream")
	}
}

func TestStreamReadAheadCloseUnblocksCommittedRead(t *testing.T) {
	source := &blockedReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
	body := newStreamReadAhead(source, time.Second)
	body.commit()
	read := make(chan error, 1)
	go func() {
		_, err := body.Read(make([]byte, 1))
		read <- err
	}()
	<-source.started
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-read:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("closed read error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing the body did not unblock its reader")
	}
}

type notifyingPipeReader struct {
	*io.PipeReader
	reads chan struct{}
}

func (reader *notifyingPipeReader) Read(buffer []byte) (int, error) {
	select {
	case reader.reads <- struct{}{}:
	default:
	}
	return reader.PipeReader.Read(buffer)
}

type blockedReadCloser struct {
	started     chan struct{}
	closed      chan struct{}
	once        sync.Once
	startedOnce sync.Once
}

func (body *blockedReadCloser) Read([]byte) (int, error) {
	body.startedOnce.Do(func() { close(body.started) })
	<-body.closed
	return 0, io.ErrClosedPipe
}

func (body *blockedReadCloser) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

var _ io.ReadCloser = (*notifyingPipeReader)(nil)
var _ io.ReadCloser = (*blockedReadCloser)(nil)

func TestProdex04361SSEFallbackDisablesPrecommitDeadline(t *testing.T) {
	reader, writer := io.Pipe()
	body := &notifyingPipeReader{PipeReader: reader, reads: make(chan struct{}, 4)}
	router := &Router{now: time.Now}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       body,
	}
	classified := make(chan struct {
		outcome responseOutcome
		pending *pendingResponse
		err     error
	}, 1)
	go func() {
		outcome, pending, err := router.inspectStreamWithPolicy(response, &pendingResponse{response: response}, "openai", 1024, 20*time.Millisecond)
		classified <- struct {
			outcome responseOutcome
			pending *pendingResponse
			err     error
		}{outcome, pending, err}
	}()
	<-body.reads
	quota := "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"usage_limit_reached\"}}}\n\n"
	_, _ = io.WriteString(writer, quota)
	got := <-classified
	if got.err != nil || got.outcome.kind != responseRetry {
		_ = writer.Close()
		got.pending.close()
		t.Fatalf("retryable response = %#v err=%v", got.outcome, got.err)
	}
	forwarded := pendingForwarded("account-a", got.outcome, got.pending)
	read := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, forwarded.Response.Body)
		read <- err
	}()
	<-body.reads
	_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\"}\n\n")
	_ = writer.Close()
	if err := <-read; err != nil {
		t.Fatalf("fallback stream read = %v", err)
	}
	_ = forwarded.Response.Body.Close()
}
