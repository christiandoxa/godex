package openai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultUpstreamURL = "https://chatgpt.com/backend-api"
	defaultListenAddr  = "127.0.0.1:0"
	defaultRequestSize = 8 << 20
	defaultInspectSize = 64 << 10
	upstreamHeaderWait = 30 * time.Second
	upstreamIdleTime   = 90 * time.Second
)

type RuntimeAccount struct {
	ID      string
	Home    string
	Enabled bool
}

type RuntimeAccountSource func(context.Context) ([]RuntimeAccount, error)

type ProxyConfig struct {
	ListenAddr       string
	UpstreamURL      string
	Accounts         RuntimeAccountSource
	PreferredAccount string
	Client           *http.Client
	Now              func() time.Time
	MaxRequestBytes  int64
	MaxInspectBytes  int64
}

type Proxy struct {
	server     *http.Server
	client     *http.Client
	source     RuntimeAccountSource
	upstream   *url.URL
	preferred  string
	now        func() time.Time
	maxRequest int64
	maxInspect int64
	affinity   *affinityStore
	listenAddr string

	mu            sync.Mutex
	listener      net.Listener
	done          chan struct{}
	endpoint      string
	cursor        int
	preferredUsed bool
	quarantine    map[string]time.Time
}

func NewProxy(config ProxyConfig) (*Proxy, error) {
	listenAddr := config.ListenAddr
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	if err := validateLoopbackAddress(listenAddr); err != nil {
		return nil, err
	}
	upstreamText := config.UpstreamURL
	if upstreamText == "" {
		upstreamText = DefaultUpstreamURL
	}
	upstream, err := url.Parse(upstreamText)
	if err != nil || upstream.Scheme == "" || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("proxy upstream URL must be an http(s) URL without credentials or query data")
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return nil, errors.New("proxy upstream URL must use http or https")
	}
	if config.Accounts == nil {
		return nil, errors.New("proxy account source is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	maxRequest := config.MaxRequestBytes
	if maxRequest <= 0 {
		maxRequest = defaultRequestSize
	}
	maxInspect := config.MaxInspectBytes
	if maxInspect <= 0 {
		maxInspect = defaultInspectSize
	}
	proxy := &Proxy{
		client:     cloneHTTPClient(config.Client),
		source:     config.Accounts,
		upstream:   upstream,
		preferred:  strings.TrimSpace(config.PreferredAccount),
		now:        now,
		maxRequest: maxRequest,
		maxInspect: maxInspect,
		affinity:   newAffinityStore(),
		quarantine: make(map[string]time.Time),
		listenAddr: listenAddr,
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	return proxy, nil
}

func (proxy *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	proxy.handle(writer, request)
}

func (proxy *Proxy) Start() error {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.listener != nil {
		return errors.New("proxy is already running")
	}
	listener, err := net.Listen("tcp", proxy.listenAddr)
	if err != nil {
		return fmt.Errorf("start loopback proxy: %w", err)
	}
	proxy.listener = listener
	proxy.endpoint = "http://" + listener.Addr().String()
	proxy.done = make(chan struct{})
	go func() {
		_ = proxy.server.Serve(listener)
		close(proxy.done)
	}()
	return nil
}

func (proxy *Proxy) Endpoint() string {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.endpoint
}

func (proxy *Proxy) Close(ctx context.Context) error {
	proxy.mu.Lock()
	server := proxy.server
	done := proxy.done
	proxy.mu.Unlock()
	if done == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
		proxy.closeIdleConnections()
		return err
	}
	select {
	case <-done:
		proxy.closeIdleConnections()
		return nil
	case <-ctx.Done():
		_ = server.Close()
		proxy.closeIdleConnections()
		return ctx.Err()
	}
}

func (proxy *Proxy) closeIdleConnections() {
	if transport, ok := proxy.client.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

func (proxy *Proxy) handle(writer http.ResponseWriter, request *http.Request) {
	body, err := readLimited(request.Body, proxy.maxRequest)
	if err != nil {
		if request.Context().Err() != nil {
			return
		}
		http.Error(writer, "request body is too large for safe retry", http.StatusRequestEntityTooLarge)
		return
	}
	keys := requestAffinity(request, body)
	now := proxy.now()
	accounts, err := proxy.source(request.Context())
	if err != nil {
		http.Error(writer, "cannot load managed accounts", http.StatusServiceUnavailable)
		return
	}
	accounts = sortRuntimeAccounts(accounts)
	owner, err := proxy.affinity.owner(keys, now)
	if err != nil {
		http.Error(writer, "request contains conflicting conversation affinity", http.StatusConflict)
		return
	}
	if owner != "" {
		proxy.handleBound(writer, request, body, keys, accounts, owner, &requestLifecycle{})
		return
	}
	candidates := proxy.candidates(accounts, now)
	if len(candidates) == 0 {
		http.Error(writer, "no enabled account is available", http.StatusServiceUnavailable)
		return
	}
	proxy.handleFresh(writer, request, body, keys, candidates, &requestLifecycle{})
}

func (proxy *Proxy) handleBound(writer http.ResponseWriter, request *http.Request, body []byte, keys affinityKeys, accounts []RuntimeAccount, owner string, lifecycle *requestLifecycle) {
	for _, account := range accounts {
		if account.ID != owner {
			continue
		}
		if !account.Enabled || proxy.isQuarantined(account.ID, proxy.now()) {
			http.Error(writer, "conversation owner is temporarily unavailable; continuity was preserved", http.StatusServiceUnavailable)
			return
		}
		response, err := proxy.doAccountRequest(request, body, account)
		if err != nil {
			if request.Context().Err() == nil {
				http.Error(writer, "conversation owner could not be reached; continuity was preserved", http.StatusBadGateway)
			}
			return
		}
		if response.StatusCode == http.StatusUnauthorized {
			proxy.quarantineAccount(account.ID, time.Minute)
		}
		proxy.forwardResponse(writer, response, nil, account.ID, keys, lifecycle)
		return
	}
	http.Error(writer, "conversation owner is no longer registered; continuity was preserved", http.StatusConflict)
}

func (proxy *Proxy) handleFresh(writer http.ResponseWriter, request *http.Request, body []byte, keys affinityKeys, candidates []RuntimeAccount, lifecycle *requestLifecycle) {
	var last *pendingResponse
	defer func() { discardPendingResponse(&last) }()
	for _, account := range candidates {
		if !lifecycle.canAttempt() {
			return
		}
		if proxy.handleFreshAttempt(writer, request, body, keys, account, lifecycle, &last) {
			return
		}
	}
	proxy.finishFresh(writer, request, keys, lifecycle, &last)
}

func (proxy *Proxy) handleFreshAttempt(writer http.ResponseWriter, request *http.Request, body []byte, keys affinityKeys, account RuntimeAccount, lifecycle *requestLifecycle, last **pendingResponse) bool {
	response, err := proxy.doAccountRequest(request, body, account)
	if err != nil {
		return request.Context().Err() != nil
	}
	outcome, pending := proxy.classify(response)
	switch outcome.kind {
	case responsePass:
		discardPendingResponse(last)
		proxy.forwardResponse(writer, response, pending.prefix, account.ID, keys, lifecycle)
		return true
	case responseRetry:
		if outcome.quarantine > 0 {
			proxy.quarantineAccount(account.ID, outcome.quarantine)
		}
		replacePendingResponse(last, pending, account.ID)
	case responseAuthFailure:
		proxy.quarantineAccount(account.ID, time.Minute)
		replacePendingResponse(last, pending, account.ID)
	}
	return false
}

func replacePendingResponse(last **pendingResponse, pending *pendingResponse, accountID string) {
	discardPendingResponse(last)
	if pending == nil {
		return
	}
	pending.accountID = accountID
	*last = pending
}

func discardPendingResponse(pending **pendingResponse) {
	if *pending == nil {
		return
	}
	(*pending).close()
	*pending = nil
}

func (proxy *Proxy) finishFresh(writer http.ResponseWriter, request *http.Request, keys affinityKeys, lifecycle *requestLifecycle, last **pendingResponse) {
	if *last != nil {
		pending := *last
		*last = nil
		proxy.forwardResponse(writer, pending.response, pending.prefix, pending.accountID, keys, lifecycle)
		return
	}
	if request.Context().Err() == nil {
		http.Error(writer, "all eligible accounts failed before upstream response commitment", http.StatusBadGateway)
	}
}

func sortRuntimeAccounts(accounts []RuntimeAccount) []RuntimeAccount {
	accounts = append([]RuntimeAccount(nil), accounts...)
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].ID != accounts[j].ID {
			return accounts[i].ID < accounts[j].ID
		}
		if accounts[i].Enabled != accounts[j].Enabled {
			return accounts[i].Enabled
		}
		return accounts[i].Home < accounts[j].Home
	})
	unique := accounts[:0]
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if _, exists := seen[account.ID]; exists {
			continue
		}
		seen[account.ID] = struct{}{}
		unique = append(unique, account)
	}
	return unique
}

func (proxy *Proxy) candidates(accounts []RuntimeAccount, now time.Time) []RuntimeAccount {
	available := make([]RuntimeAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.ID != "" && account.Home != "" && account.Enabled && !proxy.isQuarantined(account.ID, now) {
			available = append(available, account)
		}
	}
	if len(available) == 0 {
		return nil
	}
	proxy.mu.Lock()
	start := proxy.cursor % len(available)
	if !proxy.preferredUsed {
		proxy.preferredUsed = true
		for index, account := range available {
			if account.ID == proxy.preferred {
				start = index
				break
			}
		}
	}
	proxy.cursor = (start + 1) % len(available)
	proxy.mu.Unlock()
	ordered := make([]RuntimeAccount, 0, len(available))
	ordered = append(ordered, available[start:]...)
	ordered = append(ordered, available[:start]...)
	return ordered
}

func (proxy *Proxy) doAccountRequest(request *http.Request, body []byte, account RuntimeAccount) (*http.Response, error) {
	for reload := 0; reload < 2; reload++ {
		token, err := readAccessToken(filepath.Join(account.Home, "auth.json"))
		if err != nil {
			return nil, err
		}
		outgoing, err := proxy.newRequest(request, body, token)
		if err != nil {
			return nil, err
		}
		response, err := proxy.client.Do(outgoing)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized || reload == 1 {
			return response, nil
		}
		response.Body.Close()
	}
	return nil, errors.New("authentication retry failed")
}
