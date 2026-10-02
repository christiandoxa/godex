package kiro

import (
	"context"
	"errors"
	"fmt"
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
		if account.ID == "" || !account.Enabled || account.Provider.Kind != "kiro" {
			continue
		}
		transport, err := source.NewRuntimeTransport(ctx, account.Home, account.Provider.Name)
		if err != nil {
			failures++
			continue
		}
		pool.transports[account.ID] = transport
	}
	if len(pool.transports) == 0 {
		if failures > 0 {
			return nil, fmt.Errorf("no usable Kiro runtime profiles found (%d credential preparation failures)", failures)
		}
		return nil, errors.New("no enabled Kiro runtime profiles found")
	}
	return pool, nil
}

func (pool *RuntimePool) Execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if pool == nil {
		return nil, errors.New("Kiro runtime pool is unavailable")
	}
	transport := pool.transports[account.ID]
	if transport == nil {
		return nil, errors.New("Kiro runtime profile is unavailable")
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
