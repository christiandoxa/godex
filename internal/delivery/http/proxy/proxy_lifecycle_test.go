package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestRequestLifecycleAllowsRetryOnlyBeforeCommit(t *testing.T) {
	lifecycle := &requestLifecycle{}
	if !lifecycle.canAttempt() {
		t.Fatal("new request should be uncommitted")
	}
	lifecycle.commit()
	if lifecycle.canAttempt() || lifecycle.phase != requestCommitted {
		t.Fatalf("committed lifecycle = %#v", lifecycle)
	}
	lifecycle.failAfterCommit()
	if lifecycle.phase != requestFailedAfterCommit {
		t.Fatalf("failed lifecycle = %#v", lifecycle)
	}
	lifecycle.complete()
	if lifecycle.phase != requestFailedAfterCommit {
		t.Fatalf("failed lifecycle changed to %#v", lifecycle.phase)
	}

	lifecycle = &requestLifecycle{}
	lifecycle.commit()
	lifecycle.complete()
	if lifecycle.phase != requestCompleted {
		t.Fatalf("completed lifecycle = %#v", lifecycle)
	}
}

type partialFailureReader struct{ read bool }

func (reader *partialFailureReader) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, io.EOF
	}
	reader.read = true
	return copy(buffer, `{"partial":`), io.ErrUnexpectedEOF
}

func TestInspectionFailureDoesNotBecomeSuccessfulResponse(t *testing.T) {
	proxy := &Proxy{maxInspect: 64 << 10}
	writer := httptest.NewRecorder()
	lifecycle := &requestLifecycle{}
	proxy.forwardResponse(t.Context(), writer, &proxymodel.Response{
		StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(&partialFailureReader{}),
	}, nil, "one", lifecycle)
	if writer.Code != 502 || strings.Contains(writer.Body.String(), "partial") || !lifecycle.canAttempt() {
		t.Fatalf("partial response reported success: status=%d, phase=%v", writer.Code, lifecycle.phase)
	}
}

func TestManagedNonstreamReadFailureFailsBeforeCommitment(t *testing.T) {
	for _, status := range []int{200, 400, 404} {
		accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
		calls := 0
		transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(&partialFailureReader{})}, nil
		})
		proxy, err := newProxyForTest(ProxyConfig{
			Client: &http.Client{Transport: transport}, Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		writer := httptest.NewRecorder()
		proxy.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{}`)))
		proxy.router.Close()
		if writer.Code != 502 || strings.Contains(writer.Body.String(), "partial") || calls != 1 {
			t.Fatalf("HTTP %d read failure reported success or replayed: status=%d, calls=%d", status, writer.Code, calls)
		}
	}
}
