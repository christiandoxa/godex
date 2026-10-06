package copilot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type RuntimePool struct {
	transports map[string]*RuntimeTransport
	catalog    []map[string]any
}

func NewRuntimeAPIKeyPool(apiURL string, credentials []proxymodel.ProviderCredential, client *http.Client) (*RuntimePool, error) {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.githubcopilot.com"
	}
	pool := &RuntimePool{transports: make(map[string]*RuntimeTransport, len(credentials))}
	for _, credential := range credentials {
		if credential.ID == "" || credential.Secret == "" {
			pool.Close()
			return nil, errors.New("Copilot runtime API-key credential is incomplete")
		}
		transport, err := NewRuntimeTransport(apiURL, RuntimeAuth{apiKey: credential.Secret}, client)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if previous := pool.transports[credential.ID]; previous != nil {
			previous.Close()
		}
		pool.transports[credential.ID] = transport
		merged, err := mergeRuntimeCatalog(pool.catalog, transport.ModelCatalog())
		if err != nil {
			pool.Close()
			return nil, err
		}
		pool.catalog = merged
	}
	if len(pool.transports) == 0 {
		return nil, errors.New("Copilot runtime API-key pool is empty")
	}
	return pool, nil
}

func (source *Source) NewRuntimePool(ctx context.Context, accounts []proxymodel.Account) (*RuntimePool, error) {
	return source.NewRuntimePoolWithClient(ctx, accounts, source.client)
}

func (source *Source) NewRuntimePoolWithClient(ctx context.Context, accounts []proxymodel.Account, client *http.Client) (*RuntimePool, error) {
	pool := &RuntimePool{transports: make(map[string]*RuntimeTransport, len(accounts))}
	ordered := append([]proxymodel.Account(nil), accounts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	var failures int
	for _, account := range ordered {
		if account.ID == "" || !account.Enabled || account.Provider.Kind != "copilot" {
			continue
		}
		transport, err := source.NewRuntimeTransportWithClient(
			ctx, account.Provider.Host, account.Provider.Login, account.Provider.APIURL, client,
		)
		if err != nil {
			failures++
			continue
		}
		if previous := pool.transports[account.ID]; previous != nil {
			previous.Close()
		}
		pool.transports[account.ID] = transport
		merged, err := mergeRuntimeCatalog(pool.catalog, transport.ModelCatalog())
		if err != nil {
			pool.Close()
			return nil, err
		}
		pool.catalog = merged
	}
	if len(pool.transports) == 0 {
		if failures > 0 {
			return nil, fmt.Errorf("no usable Copilot runtime profiles found (%d credential resolution failures)", failures)
		}
		return nil, errors.New("no enabled Copilot runtime profiles found")
	}
	return pool, nil
}

func (pool *RuntimePool) Execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if pool == nil {
		return nil, errors.New("Copilot runtime pool is unavailable")
	}
	transport := pool.transports[account.ID]
	if transport == nil {
		return nil, errors.New("Copilot runtime profile is unavailable")
	}
	return transport.Execute(ctx, input, account)
}

func (pool *RuntimePool) AvailableAccount(id string) bool {
	return pool != nil && pool.transports[id] != nil
}

func (pool *RuntimePool) ModelCatalog() []map[string]any {
	if pool == nil {
		return nil
	}
	result := make([]map[string]any, 0, len(pool.catalog))
	for _, entry := range pool.catalog {
		result = append(result, cloneRuntimeCatalogEntry(entry))
	}
	return result
}

func (pool *RuntimePool) Close() {
	if pool == nil {
		return
	}
	for _, transport := range pool.transports {
		transport.Close()
	}
}
