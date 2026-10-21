package sse

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

type pipeSourceProbe struct {
	calls atomic.Int32
	err   error
}

func (source *pipeSourceProbe) Close() error {
	source.calls.Add(1)
	return source.err
}

func TestPipeBodyCloseRacesProducerWithoutRepeatingSourceClose(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	failure := errors.New("synthetic source close error")
	source := &pipeSourceProbe{err: failure}
	body := NewPipeBody(reader, source)
	var callers sync.WaitGroup
	for index := range 20 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			var err error
			if index%2 == 0 {
				err = body.Close()
			} else {
				err = body.CloseSource()
			}
			if !errors.Is(err, failure) {
				t.Errorf("source close error lost: %v", err)
			}
		}()
	}
	callers.Wait()
	if calls := source.calls.Load(); calls != 1 {
		t.Errorf("upstream close calls=%d want=1", calls)
	}
}

func TestPipeBodyProducerClosePreservesConsumerEOF(t *testing.T) {
	reader, writer := io.Pipe()
	source := &pipeSourceProbe{}
	body := NewPipeBody(reader, source)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer body.CloseSource()
		_, _ = writer.Write([]byte("complete"))
		_ = writer.Close()
	}()
	output, err := io.ReadAll(body)
	if err != nil || string(output) != "complete" {
		t.Fatalf("producer completion corrupted consumer drain: output=%q error=%v", output, err)
	}
	<-done
	if err := body.Close(); err != nil || source.calls.Load() != 1 {
		t.Fatalf("client close repeated producer closure: calls=%d error=%v", source.calls.Load(), err)
	}
}
