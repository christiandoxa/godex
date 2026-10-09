package routing

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	prodex04361SSECreatedPrefix = `data: {"type":"response.created","response":{"id":"held-response","padding":"`
	prodex04361SSECreatedSuffix = `"}}`

	prodex04361SSEOverload = "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\"}}}\n\n"
	prodex04361SSEOutput   = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"visible\"}\n\n"
)

// A read that returns one complete chunk per call lets the test hold the
// response after metadata without relying on a wall-clock sleep.
type parity04361ChunkReader struct {
	chunks [][]byte
}

func (reader *parity04361ChunkReader) Read(buffer []byte) (int, error) {
	if len(reader.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := reader.chunks[0]
	reader.chunks = reader.chunks[1:]
	count := copy(buffer, chunk)
	if count != len(chunk) {
		reader.chunks = append([][]byte{chunk[count:]}, reader.chunks...)
	}
	return count, nil
}

func (*parity04361ChunkReader) Close() error { return nil }

type parity04361HeldReader struct {
	prefix     []byte
	tail       []byte
	release    <-chan struct{}
	prefixDone chan struct{}
	closed     chan struct{}
	prefixOnce sync.Once
	closeOnce  sync.Once
}

func (reader *parity04361HeldReader) Read(buffer []byte) (int, error) {
	if len(reader.prefix) > 0 {
		count := copy(buffer, reader.prefix)
		reader.prefix = reader.prefix[count:]
		if len(reader.prefix) == 0 {
			reader.prefixOnce.Do(func() { close(reader.prefixDone) })
		}
		return count, nil
	}
	select {
	case <-reader.release:
	case <-reader.closed:
		return 0, io.ErrClosedPipe
	}
	if len(reader.tail) == 0 {
		return 0, io.EOF
	}
	count := copy(buffer, reader.tail)
	reader.tail = reader.tail[count:]
	return count, nil
}

func (reader *parity04361HeldReader) Close() error {
	reader.closeOnce.Do(func() { close(reader.closed) })
	return nil
}

func TestProdex04361DelayedCapacityAfterLargeMetadataStaysPrecommit(t *testing.T) {
	prefix := []byte(prodex04361SSECreatedPrefix + strings.Repeat("x", 9_000) + prodex04361SSECreatedSuffix + "\n\n")
	allowTail := make(chan struct{})
	reader := &parity04361HeldReader{
		prefix: prefix, tail: []byte(prodex04361SSEOverload),
		release: allowTail, prefixDone: make(chan struct{}), closed: make(chan struct{}),
	}
	router := &Router{
		now: time.Now, inflight: make(map[string]int), inflightChanged: make(chan struct{}),
		profileInflightHardLimit: 1,
	}
	releaseAdmission, acquired := router.tryAcquireProfileInflight("account-a", proxymodel.Request{}, false)
	if !acquired {
		t.Fatal("profile admission was rejected")
	}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       &profileInflightBody{ReadCloser: reader, release: releaseAdmission},
	}
	type result struct {
		outcome responseOutcome
		pending *pendingResponse
		err     error
	}
	results := make(chan result, 1)
	go func() {
		outcome, pending, err := router.classify(response, "openai")
		results <- result{outcome: outcome, pending: pending, err: err}
	}()

	<-reader.prefixDone
	select {
	case got := <-results:
		got.pending.close()
		t.Fatalf("large metadata committed before delayed overload: %#v", got)
	default:
	}
	close(allowTail)
	got := <-results
	defer got.pending.close()
	if got.err != nil || got.outcome.kind != responseRetry || !got.outcome.transient {
		t.Fatalf("delayed overload outcome = %#v, err=%v", got.outcome, got.err)
	}
	router.mu.Lock()
	active := router.inflight["account-a"]
	router.mu.Unlock()
	if active != 0 {
		t.Fatalf("failed SSE attempt retained profile admission: %d", active)
	}
}

func TestProdex04361VisibleOutputCommitsBeforeLaterCapacityFailure(t *testing.T) {
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body: &parity04361ChunkReader{
			chunks: [][]byte{[]byte(prodex04361SSEOutput), []byte(prodex04361SSEOverload)},
		},
	}
	router := &Router{now: time.Now}
	outcome, pending, err := router.classify(response, "openai")
	if err != nil || outcome.kind != responsePass || outcome.failed {
		pending.close()
		t.Fatalf("visible output outcome = %#v, err=%v", outcome, err)
	}
	if string(pending.prefix) != prodex04361SSEOutput {
		pending.close()
		t.Fatalf("visible prefix = %q, want first output event only", pending.prefix)
	}
	pending.commitStream()
	tail, err := io.ReadAll(pending.response.Body)
	pending.close()
	if err != nil || string(tail) != prodex04361SSEOverload {
		t.Fatalf("committed tail = %q, err=%v", tail, err)
	}
}
