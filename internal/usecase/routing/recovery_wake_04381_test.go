package routing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestFreshRecoveryWakeOnSelectionChange(t *testing.T) {
	var loads atomic.Int32
	waitEntered := make(chan struct{})
	router, err := NewRouter(Config{
		Gateway: &sequenceRoutingGateway{},
		Wait: func(ctx context.Context, _ time.Duration) error {
			select {
			case <-waitEntered:
			default:
				close(waitEntered)
			}
			<-ctx.Done()
			return ctx.Err()
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			loads.Add(1)
			return []proxymodel.Account{{ID: "account-a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("account-a", 10*time.Second)

	done := make(chan error, 1)
	started := time.Now()
	go func() {
		exchange, err := router.Forward(context.Background(), proxymodel.Request{})
		if exchange != nil {
			_ = exchange.Close()
		}
		done <- err
	}()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("recovery wait did not start")
	}
	router.clearQuarantine("account-a")

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selection change did not wake recovery")
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Fatalf("recovery wake took %s", elapsed)
	}
	if loads.Load() < 2 {
		t.Fatalf("account loads = %d, want a post-wake reload", loads.Load())
	}
}
