package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestProdex04380CapturedClosingRequestIgnoresDelayedReadEOF(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "fixture-a", "B", "fixture-b")
	for _, closing := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			calls := 0
			transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				started <- request.Context()
				<-release
				if err := request.Context().Err(); err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusMultiStatus, Header: http.Header{},
					Body: io.NopCloser(strings.NewReader(`{"reply":"ok"}`))}, nil
			})
			proxy, err := newProxyForTest(ProxyConfig{Client: &http.Client{Transport: transport},
				Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer proxy.router.Close()
			request := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{}`))
			request.Close = closing
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			writer := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { proxy.ServeHTTP(writer, request.WithContext(ctx)); close(done) }()
			upstreamContext := <-started
			// Virtual time proves the old 1 ms heuristic expired; no wall-clock sleep.
			time.Sleep(time.Second)
			cancel()
			synctest.Wait()
			upstreamErr := upstreamContext.Err()
			close(release)
			<-done
			if calls != 1 {
				t.Fatalf("captured request was replayed: %d attempts", calls)
			}
			if !closing && !errors.Is(upstreamErr, context.Canceled) {
				t.Fatalf("ordinary request lost cancellation: %v", upstreamErr)
			}
			if closing && (upstreamErr != nil || writer.Code != http.StatusMultiStatus || writer.Body.String() != `{"reply":"ok"}`) {
				t.Fatalf("captured closing request lost response: err=%v status=%d body=%q", upstreamErr, writer.Code, writer.Body.String())
			}
		})
	}
}
