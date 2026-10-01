package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type Config struct {
	Router                           *routingusecase.Router
	Activity                         activityRecorder
	ListenAddr                       string
	MaxRequestBytes, MaxInspectBytes int64
}
type Proxy struct {
	router                 *routingusecase.Router
	server                 *http.Server
	listenAddr             string
	maxRequest, maxInspect int64
	mu                     sync.Mutex
	sequence               atomic.Uint64
	activity               activityRecorder
	listener               net.Listener
	done                   chan struct{}
	endpoint               string
}

func NewProxy(config Config) (*Proxy, error) {
	if config.Router == nil {
		return nil, errors.New("proxy routing use case is required")
	}
	if config.ListenAddr == "" {
		config.ListenAddr = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(config.ListenAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("proxy listen address must be loopback (127.0.0.1 or ::1)")
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = 8 << 20
	}
	if config.MaxInspectBytes <= 0 {
		config.MaxInspectBytes = 64 << 10
	}
	proxy := &Proxy{router: config.Router, activity: config.Activity, listenAddr: config.ListenAddr, maxRequest: config.MaxRequestBytes, maxInspect: config.MaxInspectBytes}
	proxy.server = &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	return proxy, nil
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
	go func() { _ = proxy.server.Serve(listener); close(proxy.done) }()
	return nil
}
func (proxy *Proxy) Endpoint() string {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.endpoint
}
func (proxy *Proxy) Close(ctx context.Context) error {
	defer proxy.router.Close()
	proxy.mu.Lock()
	done := proxy.done
	proxy.mu.Unlock()
	if done == nil {
		return nil
	}
	if err := proxy.server.Shutdown(ctx); err != nil {
		_ = proxy.server.Close()
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		_ = proxy.server.Close()
		return ctx.Err()
	}
}
func (proxy *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	activity := proxy.startActivity(request)
	activityContext := context.WithoutCancel(request.Context())
	defer proxy.finishActivity(activityContext, activity)
	if request.Header.Get("Upgrade") != "" {
		activity.fail(http.StatusUpgradeRequired, "websocket upgrade is not supported")
		http.Error(writer, "Godex requires Codex HTTP/SSE model transport", http.StatusUpgradeRequired)
		return
	}
	body, err := readLimited(request.Body, proxy.maxRequest)
	if err != nil {
		if request.Context().Err() == nil {
			activity.fail(http.StatusRequestEntityTooLarge, "request body exceeded safe retry limit")
			http.Error(writer, "request body is too large for safe retry", http.StatusRequestEntityTooLarge)
		} else {
			activity.fail(0, "request canceled")
		}
		return
	}
	exchange, err := proxy.router.Forward(request.Context(), proxymodel.Request{Method: request.Method, Path: request.URL.Path, RawPath: request.URL.EscapedPath(), RawQuery: request.URL.RawQuery, Header: request.Header.Clone(), Body: body})
	if err != nil {
		if request.Context().Err() != nil {
			activity.fail(0, "request canceled")
			return
		}
		status, message := http.StatusBadGateway, "managed request failed before response commitment"
		var presentation *proxymodel.Error
		if errors.As(err, &presentation) {
			status, message = presentation.StatusCode, presentation.Message
		}
		activity.fail(status, message)
		http.Error(writer, message, status)
		return
	}
	defer exchange.Close()
	result := exchange.Result
	activity.upstream(result.AccountID, result.Response.StatusCode)
	if result.Failed {
		result.AccountID = ""
	}
	lifecycle := &requestLifecycle{}
	defer activity.finishLifecycle(lifecycle)
	proxy.forwardResponse(request.Context(), writer, result.Response, result.Prefix, result.AccountID, lifecycle)
}

func readLimited(reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("request body limit exceeded")
	}
	return data, nil
}
