package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type RuntimePool struct {
	transports map[string]*RuntimeTransport
}

func (source *Source) NewRuntimePool(ctx context.Context, accounts []proxymodel.Account) (*RuntimePool, error) {
	pool := &RuntimePool{transports: make(map[string]*RuntimeTransport, len(accounts))}
	ordered := append([]proxymodel.Account(nil), accounts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	failures := 0
	for _, account := range ordered {
		if account.ID == "" || !account.Enabled || account.Provider.Kind != "anthropic" {
			continue
		}
		transport, err := source.NewRuntimeTransport(ctx, account.Home, account.Provider.APIURL, nil)
		if err != nil {
			failures++
			continue
		}
		if previous := pool.transports[account.ID]; previous != nil {
			previous.Close()
		}
		pool.transports[account.ID] = transport
	}
	if len(pool.transports) == 0 {
		if failures > 0 {
			return nil, fmt.Errorf("no usable Anthropic runtime profiles found (%d credential resolution failures)", failures)
		}
		return nil, errors.New("no enabled Anthropic runtime profiles found")
	}
	return pool, nil
}

func (pool *RuntimePool) Execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if pool == nil {
		return nil, errors.New("Anthropic runtime pool is unavailable")
	}
	transport := pool.transports[account.ID]
	if transport == nil {
		return nil, errors.New("Anthropic runtime profile is unavailable")
	}
	return transport.Execute(ctx, input, account)
}

func (pool *RuntimePool) AvailableAccount(id string) bool {
	return pool != nil && pool.transports[id] != nil
}

func (pool *RuntimePool) Close() {
	if pool == nil {
		return
	}
	for _, transport := range pool.transports {
		transport.Close()
	}
}

func NewRuntimeAPIKeyPool(
	apiURL string,
	credentials []proxymodel.ProviderCredential,
	client *http.Client,
) (*RuntimePool, error) {
	pool := &RuntimePool{transports: make(map[string]*RuntimeTransport, len(credentials))}
	for _, credential := range credentials {
		if credential.ID == "" || credential.Secret == "" {
			pool.Close()
			return nil, errors.New("Anthropic runtime API-key credential is incomplete")
		}
		transport, err := newRuntimeAPIKeyTransport(apiURL, credential.Secret, client)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if previous := pool.transports[credential.ID]; previous != nil {
			previous.Close()
		}
		pool.transports[credential.ID] = transport
	}
	if len(pool.transports) == 0 {
		return nil, errors.New("Anthropic runtime API-key pool is empty")
	}
	return pool, nil
}
