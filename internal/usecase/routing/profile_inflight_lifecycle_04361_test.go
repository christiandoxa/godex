package routing

import (
	"errors"
	"io"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProfileInflightBodyReleasesOnReadReturningBytesAndEOF(t *testing.T) {
	router := &Router{
		inflight:                 make(map[string]int),
		inflightChanged:          make(chan struct{}),
		profileInflightHardLimit: 1,
	}
	release, acquired := router.tryAcquireProfileInflight(
		"account-a", proxymodel.Request{}, false,
	)
	if !acquired {
		t.Fatal("profile admission was rejected")
	}
	body := &profileInflightBody{
		ReadCloser: terminalReadCloser{},
		release:    release,
	}
	buffer := make([]byte, 1)
	count, err := body.Read(buffer)
	if count != 1 || !errors.Is(err, io.EOF) {
		t.Fatalf("terminal read = (%d, %v), want one byte and EOF", count, err)
	}
	router.mu.Lock()
	got := router.inflight["account-a"]
	router.mu.Unlock()
	if got != 0 {
		t.Fatalf("in-flight count after terminal read = %d, want 0", got)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("close = %v", err)
	}
	if router.profileInflightReleasesTotal != 1 || router.profileInflightReleaseUnderflowsTotal != 0 {
		t.Fatalf("release metrics = %d/%d, want 1/0", router.profileInflightReleasesTotal, router.profileInflightReleaseUnderflowsTotal)
	}
}

type terminalReadCloser struct{}

func (terminalReadCloser) Read(buffer []byte) (int, error) {
	buffer[0] = 'x'
	return 1, io.EOF
}

func (terminalReadCloser) Close() error { return nil }
