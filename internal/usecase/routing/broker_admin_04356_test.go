package routing

import (
	"context"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type brokerReleaseRepository struct {
	bindings map[string]routingentity.Binding
}

func (repo *brokerReleaseRepository) Load(context.Context) ([]routingentity.Binding, error) {
	result := make([]routingentity.Binding, 0, len(repo.bindings))
	for _, binding := range repo.bindings {
		result = append(result, binding)
	}
	return result, nil
}
func (repo *brokerReleaseRepository) Merge(_ context.Context, values []routingentity.Binding) ([]routingentity.Binding, error) {
	for _, value := range values {
		repo.bindings[value.Key] = value
	}
	return values, nil
}
func (repo *brokerReleaseRepository) Remove(_ context.Context, keys []string) error {
	for _, key := range keys {
		delete(repo.bindings, key)
	}
	return nil
}
func (*brokerReleaseRepository) AcquireConversation(context.Context) (func() error, error) {
	return func() error { return nil }, nil
}

func TestProdex04356BrokerReleaseSessionAffinityClearsRegularAndCompact(t *testing.T) {
	repo := &brokerReleaseRepository{bindings: make(map[string]routingentity.Binding)}
	router, err := NewRouter(Config{
		Gateway: &retryBackoffGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Enabled: true}}, nil
		},
		Bindings: repo,
		Now:      func() time.Time { return time.Unix(100, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	now := time.Unix(100, 0)
	for _, session := range []string{"session-a", "__compact_session__:session-a"} {
		if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{session: session}, now); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.bindings) != 2 {
		t.Fatalf("seeded bindings = %d, want 2", len(repo.bindings))
	}
	if err := router.ReleaseSessionAffinity(t.Context(), " session-a "); err != nil {
		t.Fatal(err)
	}
	if len(repo.bindings) != 0 {
		t.Fatalf("broker release left bindings: %#v", repo.bindings)
	}
}
