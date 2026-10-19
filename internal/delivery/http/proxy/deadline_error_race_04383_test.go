package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type expiringErrorContext04383 struct {
	context.Context
	done        chan struct{}
	armed       atomic.Bool
	classifying atomic.Bool
	expired     atomic.Bool
	once        sync.Once
}

func (ctx *expiringErrorContext04383) Done() <-chan struct{} { return ctx.done }

func (ctx *expiringErrorContext04383) expire() {
	ctx.once.Do(func() { ctx.expired.Store(true); close(ctx.done) })
}

func (ctx *expiringErrorContext04383) Err() error {
	if ctx.expired.Load() {
		return context.DeadlineExceeded
	}
	// Expire immediately after the first deadline-classification snapshot.
	if ctx.armed.Load() && ctx.classifying.Load() {
		ctx.expire()
	}
	return nil
}

func (ctx *expiringErrorContext04383) Value(key any) any {
	if _, ok := key.(requestDeadlineContextKey); ok {
		if ctx.armed.Load() {
			ctx.classifying.Store(true)
		}
		return requestDeadlineScope{parent: context.Background(), deadline: time.Now().Add(time.Hour), local: true}
	}
	return ctx.Context.Value(key)
}

func TestProdex04383DeadlineBetweenErrorChecksNeverReturnsEmpty200(t *testing.T) {
	for _, path := range []string{"/backend-api/godex/responses", "/backend-api/godex/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			ctx := &expiringErrorContext04383{Context: context.Background(), done: make(chan struct{})}
			t.Cleanup(ctx.expire)
			router, err := routingusecase.NewRouter(routingusecase.Config{
				PrecommitWatchdogBudget: 45 * time.Millisecond,
				Accounts: func(waiting context.Context) ([]proxymodel.Account, error) {
					<-waiting.Done()
					ctx.armed.Store(true)
					return nil, waiting.Err()
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			proxy, err := NewProxy(Config{Router: router})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
			response := httptest.NewRecorder()
			proxy.ServeHTTP(response, request)
			if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), `"code":"request_timeout"`) {
				t.Fatalf("deadline/error race returned %d/%q", response.Code, response.Body.String())
			}
		})
	}
}
