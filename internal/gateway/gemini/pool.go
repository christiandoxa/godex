package gemini

import (
	"context"
	"errors"
	"net/http"

	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type RuntimePool struct {
	transports map[string]*RuntimeTransport
}

func NewRuntimePool(apiURL string, credentials []proxymodel.ProviderCredential, client *http.Client) (*RuntimePool, error) {
	return newRuntimePool(apiURL, credentials, client, true)
}

// NewLocalRuntimePool retains history across session labels within one local invocation.
func NewLocalRuntimePool(apiURL string, credentials []proxymodel.ProviderCredential, client *http.Client) (*RuntimePool, error) {
	return newRuntimePool(apiURL, credentials, client, false)
}

func newRuntimePool(apiURL string, credentials []proxymodel.ProviderCredential, client *http.Client, scopeConversations bool) (*RuntimePool, error) {
	pool := &RuntimePool{transports: make(map[string]*RuntimeTransport, len(credentials))}
	for _, credential := range credentials {
		if credential.ID == "" || credential.Secret == "" {
			pool.Close()
			return nil, errors.New("Gemini runtime API-key credential is incomplete")
		}
		transport, err := NewRuntimeTransport(apiURL, credential.Secret, client)
		if err != nil {
			pool.Close()
			return nil, err
		}
		// Tagged Prodex 0.437.11 Super API-key Gemini routes use the
		// OpenAI-compatible chat bridge. Direct Gemini native transports
		// retain generateContent for their separate provider surface.
		transport.chatCompatible = true
		transport.scopeConversations = scopeConversations
		if previous := pool.transports[credential.ID]; previous != nil {
			previous.Close()
		}
		pool.transports[credential.ID] = transport
	}
	if len(pool.transports) == 0 {
		return nil, errors.New("Gemini runtime API-key pool is empty")
	}
	return pool, nil
}

func (pool *RuntimePool) Execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if pool == nil {
		return nil, errors.New("Gemini runtime pool is unavailable")
	}
	transport := pool.transports[account.ID]
	if transport == nil {
		return nil, errors.New("Gemini runtime credential is unavailable")
	}
	return transport.Execute(ctx, request, account)
}

func (pool *RuntimePool) AvailableAccount(id string) bool {
	return pool != nil && pool.transports[id] != nil
}

func (pool *RuntimePool) LocalCompactFallback(body []byte, reason string) (*proxymodel.Response, error) {
	return compactgateway.LocalFallback(body, "gemini", reason)
}

func (pool *RuntimePool) Close() {
	if pool == nil {
		return
	}
	for _, transport := range pool.transports {
		transport.Close()
	}
}
