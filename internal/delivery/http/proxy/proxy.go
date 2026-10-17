package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type BodyRedactor interface {
	Redact(context.Context, []byte) ([]byte, error)
}

type Config struct {
	Router                           *routingusecase.Router
	Activity                         activityRecorder
	Broker                           *proxymodel.BrokerConfig
	ListenAddr                       string
	SmartContextEnabled              bool
	Redactor                         BodyRedactor
	ActiveRequestLimit               int
	AdmissionWaitBudget              time.Duration
	PressureAdmissionWaitBudget      time.Duration
	LongLivedQueueWaitBudget         time.Duration
	PressureLongLivedQueueWaitBudget time.Duration
	PressureSnapshot                 func() AdmissionPressure
	MaxRequestBytes, MaxInspectBytes int64
}
type Proxy struct {
	router                    *routingusecase.Router
	server                    *http.Server
	listenAddr                string
	requestBudget             time.Duration
	maxRequest, maxInspect    int64
	mu                        sync.Mutex
	sequence                  atomic.Uint64
	activeRequests            atomic.Int64
	activity                  activityRecorder
	broker                    *proxymodel.BrokerConfig
	brokerLog                 *brokerLiveLog
	smartContextEnabled       bool
	redactor                  BodyRedactor
	pressureSnapshot          func() AdmissionPressure
	admission                 *activeRequestHandler
	listener                  net.Listener
	done                      chan struct{}
	endpoint                  string
	tunnels                   map[*websocketTunnel]struct{}
	responsesWebSocketTunnels map[*responsesWebSocketTunnel]struct{}
	closing                   bool
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
	proxy := &Proxy{
		router: config.Router, activity: config.Activity, broker: config.Broker, listenAddr: config.ListenAddr,
		requestBudget: config.Router.RequestWatchdogBudget(),
		maxRequest:    config.MaxRequestBytes, maxInspect: config.MaxInspectBytes,
		smartContextEnabled:       config.SmartContextEnabled,
		redactor:                  config.Redactor,
		pressureSnapshot:          config.PressureSnapshot,
		tunnels:                   make(map[*websocketTunnel]struct{}),
		responsesWebSocketTunnels: make(map[*responsesWebSocketTunnel]struct{}),
	}
	if config.Broker != nil {
		proxy.brokerLog = &brokerLiveLog{}
	}
	handler := newActiveRequestHandlerWithRecorder(proxy, config.ActiveRequestLimit, config.Activity)
	if admission, ok := handler.(*activeRequestHandler); ok {
		proxy.admission = admission
		admission.requestBudget = config.Router.RequestWatchdogBudget()
		admission.admissionWaitBudget = positiveWaitBudget(config.AdmissionWaitBudget, admissionWaitInterval)
		admission.pressureAdmissionWaitBudget = positiveWaitBudget(config.PressureAdmissionWaitBudget, pressureAdmissionWaitInterval)
		admission.longLivedQueueWaitBudget = positiveWaitBudget(config.LongLivedQueueWaitBudget, admissionWaitInterval)
		admission.pressureLongLivedQueueWaitBudget = positiveWaitBudget(config.PressureLongLivedQueueWaitBudget, pressureAdmissionWaitInterval)
		config.Router.SetAdmissionSelectionNotifier(admission.notifySelectionChange)
	}
	proxy.server = &http.Server{
		Handler: handler, ConnContext: proxyConnContext,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second,
	}
	return proxy, nil
}

func positiveWaitBudget(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
func (proxy *Proxy) Start() error {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.closing {
		return errors.New("proxy is closed")
	}
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
	if proxy.brokerLog != nil {
		proxy.brokerLog.append(fmt.Sprintf(
			"runtime_broker_started listen_addr=%s current_profile=%s",
			listener.Addr().String(), proxy.broker.CurrentProfile,
		))
	}
	go func() { _ = serveProxyListener(proxy.server, listener); close(proxy.done) }()
	return nil
}

func (proxy *Proxy) SetPersistenceEnabled(enabled bool) {
	if proxy == nil || proxy.router == nil {
		return
	}
	proxy.router.SetPersistenceEnabled(enabled)
}

func (proxy *Proxy) SetBrokerPersistenceRole(role string) {
	if proxy == nil {
		return
	}
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.broker != nil {
		proxy.broker.PersistenceRole = strings.TrimSpace(role)
	}
}

func (proxy *Proxy) RecordBrokerLog(line string) {
	if proxy == nil || proxy.brokerLog == nil {
		return
	}
	proxy.brokerLog.append(line)
}

func (proxy *Proxy) ActiveRequests() int {
	if proxy == nil {
		return 0
	}
	return int(proxy.activeRequests.Load())
}

func (proxy *Proxy) Endpoint() string {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.endpoint
}

func (proxy *Proxy) Close(ctx context.Context) error {
	defer proxy.router.Close()
	proxy.mu.Lock()
	proxy.closing = true
	done := proxy.done
	tunnels := make([]*websocketTunnel, 0, len(proxy.tunnels))
	for tunnel := range proxy.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	responsesTunnels := make([]*responsesWebSocketTunnel, 0, len(proxy.responsesWebSocketTunnels))
	for tunnel := range proxy.responsesWebSocketTunnels {
		responsesTunnels = append(responsesTunnels, tunnel)
	}
	proxy.mu.Unlock()
	for _, tunnel := range tunnels {
		tunnel.close()
	}
	for _, tunnel := range responsesTunnels {
		tunnel.close()
	}
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
	if proxy.handleBrokerAdmin(writer, request) {
		return
	}
	proxy.activeRequests.Add(1)
	defer proxy.activeRequests.Add(-1)
	requestContext, cancel := withRequestDeadline(request.Context(), proxy.requestBudget)
	defer cancel()
	request = request.WithContext(requestContext)
	activity := proxy.startActivity(request)
	activityContext := context.WithoutCancel(request.Context())
	defer proxy.finishActivity(activityContext, activity)
	var detachCancel context.CancelFunc = func() {}
	defer func() { detachCancel() }()
	lifecycle := &requestLifecycle{}
	websocket := isWebSocketUpgradeRequest(request)
	websocketKey := ""
	var body []byte
	if websocket {
		if status, message := websocketRequestError(request); status != 0 {
			activity.fail(status, message)
			http.Error(writer, message, status)
			return
		}
		websocketKey = websocketRequestKey(request)
		if websocketUsesMessageRouting(request.URL.Path) {
			if err := proxy.forwardResponsesWebSocket(writer, request, activity, lifecycle, websocketKey); err != nil {
				if lifecycle.canAttempt() {
					activity.fail(http.StatusBadGateway, "websocket handshake failed before commitment")
					http.Error(writer, "websocket handshake failed", http.StatusBadGateway)
				} else {
					lifecycle.failAfterCommit()
				}
			}
			if !lifecycle.canAttempt() {
				activity.finishLifecycle(lifecycle)
			}
			return
		}
	} else {
		if request.ContentLength > proxy.maxRequest {
			const message = "proxied request body is too large"
			activity.fail(http.StatusRequestEntityTooLarge, message)
			writeTextResponse(writer, http.StatusRequestEntityTooLarge, message)
			return
		}
		var err error
		body, err = readLimited(requestContext, request.Body, proxy.maxRequest)
		if err != nil {
			if requestDeadlineTimedOut(requestContext) {
				activity.fail(http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
				writeProxyError(writer, request.URL.Path, http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
				return
			}
			if request.Context().Err() != nil {
				activity.fail(0, "request canceled")
				return
			}
			status, message := http.StatusBadGateway, "proxied request could not be captured"
			if errors.Is(err, errRequestBodyTooLarge) {
				status, message = http.StatusRequestEntityTooLarge, "proxied request body is too large"
			}
			activity.fail(status, message)
			writeTextResponse(writer, status, message)
			return
		}
		// A closing server connection can report read EOF at any point after
		// capture. Finish its complete request like the reference runtime;
		// body-read failures and ordinary request cancellation still stop work.
		if requestDeadlineTimedOut(requestContext) {
			activity.fail(http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
			writeProxyError(writer, request.URL.Path, http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
			return
		}
		if requestContext.Err() != nil || request.RequestURI != "" && request.Close {
			requestContext, detachCancel = detachRequestDeadline(requestContext)
		}
		if proxy.redactor != nil && len(body) > 0 {
			redacted, err := proxy.redactor.Redact(requestContext, body)
			if err != nil {
				activity.fail(http.StatusBadGateway, "presidio_redaction_failed")
				writeTextResponse(writer, http.StatusBadGateway, "gateway PII redaction failed")
				return
			}
			body = redacted
		}
		bodyBytesBeforeSmartContext := len(body)
		smart := prepareSmartContextHTTPBody(
			proxy.smartContextEnabled, request.URL.Path, request.Header, body,
		)
		proxy.recordSmartContextResult(
			context.WithoutCancel(request.Context()),
			activity.sequence,
			request.URL.Path,
			false,
			bodyBytesBeforeSmartContext,
			smart,
		)
		body = smart.Body
	}
	exchange, err := proxy.router.Forward(requestContext, proxymodel.Request{
		RequestID: activity.sequence, Method: request.Method, Path: request.URL.Path,
		RawPath: request.URL.EscapedPath(), RawQuery: request.URL.RawQuery,
		Header: request.Header.Clone(), Body: body,
		QuotaSelection: proxy.quotaSelection(request.URL.Path, websocket, body),
	})
	if err != nil {
		if requestDeadlineTimedOut(requestContext) {
			activity.fail(http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
			writeProxyError(writer, request.URL.Path, http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
			return
		}
		if requestContext.Err() != nil {
			activity.fail(0, "request canceled")
			return
		}
		status, message := http.StatusBadGateway, "managed request failed before response commitment"
		var presentation *proxymodel.Error
		if errors.As(err, &presentation) {
			status, message = presentation.StatusCode, presentation.Message
		}
		activity.fail(status, message)
		writeProxyError(writer, request.URL.Path, status, message)
		return
	}
	defer exchange.Close()
	result := exchange.Result
	activity.upstream(result.AccountID, result.Response.StatusCode)
	if result.Failed {
		result.AccountID = ""
	}
	defer activity.finishLifecycle(lifecycle)
	if websocket && result.Response.StatusCode == http.StatusSwitchingProtocols {
		if err := proxy.forwardWebSocket(writer, result.Response, lifecycle, websocketKey); err != nil {
			if lifecycle.canAttempt() {
				activity.fail(http.StatusBadGateway, "upstream websocket handshake failed before commitment")
			} else {
				lifecycle.failAfterCommit()
			}
		}
		return
	}
	if websocket {
		result.Response.Header = result.Response.Header.Clone()
		result.Response.Header.Del("Set-Cookie")
	}
	proxy.forwardResponse(requestContext, writer, result.Response, result.Prefix, result.AccountID, lifecycle, result.ProviderKind)
}

var errRequestBodyTooLarge = errors.New("request body limit exceeded")

func readLimited(ctx context.Context, reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(reader, limit+1))
		read <- result{data: data, err: err}
	}()
	select {
	case result := <-read:
		_ = reader.Close()
		if result.err != nil {
			return nil, result.err
		}
		if int64(len(result.data)) > limit {
			return nil, errRequestBodyTooLarge
		}
		return result.data, nil
	case <-ctx.Done():
		_ = reader.Close()
		return nil, ctx.Err()
	}
}
