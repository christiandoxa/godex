package routing

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const (
	defaultProfileInflightHardLimit = 8
	defaultProfileInflightWaitEpoch = 30 * time.Second
)

type profileInflightWaitFunc func(context.Context, <-chan struct{}, time.Duration) error

func waitProfileInflightSignalOrEpoch(ctx context.Context, changed <-chan struct{}, epoch time.Duration) error {
	timer := time.NewTimer(epoch)
	defer timer.Stop()
	select {
	case <-changed:
		return nil
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func requestProfileInflightWeight(request proxymodel.Request) int {
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") {
		return 2
	}
	path := strings.TrimRight(request.Path, "/")
	if strings.HasSuffix(path, "/responses") && !strings.HasSuffix(path, "/responses/compact") {
		return 2
	}
	return 1
}

func effectiveProfileInflightHardLimit(configured, weight int) int {
	if configured < weight {
		return weight
	}
	return configured
}

func (router *Router) tryAcquireProfileInflight(
	accountID string,
	request proxymodel.Request,
	hardAffinity bool,
) (func(), bool) {
	weight := requestProfileInflightWeight(request)
	router.mu.Lock()
	current := router.inflight[accountID]
	hardLimit := effectiveProfileInflightHardLimit(router.profileInflightHardLimit, weight)
	if !hardAffinity && current > hardLimit-weight {
		router.mu.Unlock()
		transport := "http"
		if strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") || request.WebSocketMessage {
			transport = "websocket"
		}
		fields := map[string]string{
			"profile": accountID, "hard_limit": strconv.Itoa(hardLimit),
			"route": routeHealthRoute(request.QuotaSelection.RouteKind), "transport": transport,
			"active": strconv.Itoa(current), "weight": strconv.Itoa(weight),
		}
		event := runtimemodel.Event{Kind: "profile_inflight_saturated", Fields: fields}
		if request.RequestID != 0 {
			event.RequestID = strconv.FormatUint(request.RequestID, 10)
		}
		router.recordRuntimeMarker(context.Background(), event)
		return nil, false
	}
	maxInt := int(^uint(0) >> 1)
	next := maxInt
	if current <= maxInt-weight {
		next = current + weight
	}
	router.inflight[accountID] = next
	router.profileInflightAdmissionsTotal++
	router.mu.Unlock()
	router.recordProfileInflightMarker(accountID, next)

	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			current := router.inflight[accountID]
			active := 0
			if current <= 0 {
				router.profileInflightReleaseUnderflowsTotal++
			} else {
				router.profileInflightReleasesTotal++
				if current <= weight {
					delete(router.inflight, accountID)
				} else {
					active = current - weight
					router.inflight[accountID] = active
				}
			}
			close(router.inflightChanged)
			router.inflightChanged = make(chan struct{})
			router.mu.Unlock()
			router.recordProfileInflightMarker(accountID, active)
		})
	}, true
}

// waitForProfileInflight rechecks capacity under the same lock protecting the
// permit counter and generation. A release between a rejected acquisition and
// waiter registration must not strand an unsent request until the next epoch.
func (router *Router) waitForProfileInflight(
	ctx context.Context,
	request proxymodel.Request,
	candidates []proxymodel.Account,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	weight := requestProfileInflightWeight(request)
	router.mu.Lock()
	limit := effectiveProfileInflightHardLimit(router.profileInflightHardLimit, weight)
	for _, candidate := range candidates {
		if router.inflight[candidate.ID] <= limit-weight {
			router.mu.Unlock()
			return nil
		}
	}
	changed := router.inflightChanged
	wait := router.profileInflightWait
	router.mu.Unlock()
	return wait(ctx, changed, defaultProfileInflightWaitEpoch)
}

func (router *Router) tryExecuteWithProfileInflight(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	hardAffinity bool,
) (*proxymodel.Response, bool, error) {
	release, acquired := router.tryAcquireProfileInflight(account.ID, request, hardAffinity)
	if !acquired {
		return nil, false, nil
	}
	var response *proxymodel.Response
	var err error
	if request.WebSocketMessage {
		response, err = router.executeRouted(ctx, request, account, hardAffinity)
	} else {
		response, err = router.executeAccount(ctx, request, account)
	}
	if err != nil {
		release()
		return nil, true, err
	}
	if response == nil || response.Body == nil {
		release()
		return response, true, nil
	}
	body := &profileInflightBody{ReadCloser: response.Body, release: release}
	if duplex, ok := response.Body.(io.ReadWriteCloser); ok {
		response.Body = &profileInflightDuplexBody{profileInflightBody: body, duplex: duplex}
	} else {
		response.Body = body
	}
	return response, true, nil
}

func (router *Router) executeWithProfileInflightWait(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	hardAffinity bool,
) (*proxymodel.Response, error) {
	for {
		response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, account, hardAffinity)
		if err != nil || acquired {
			return response, err
		}
		if err := router.waitForProfileInflight(ctx, request, []proxymodel.Account{account}); err != nil {
			return nil, err
		}
	}
}

type profileInflightBody struct {
	io.ReadCloser
	release func()
}

func (body *profileInflightBody) releaseAdmission() { body.release() }

func releaseProfileInflight(body io.ReadCloser) {
	if releasable, ok := body.(interface{ releaseAdmission() }); ok {
		releasable.releaseAdmission()
	}
}

type profileInflightDuplexBody struct {
	*profileInflightBody
	duplex io.ReadWriteCloser
}

func (body *profileInflightDuplexBody) Write(buffer []byte) (int, error) {
	return body.duplex.Write(buffer)
}

func (body *profileInflightBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if err == io.EOF {
		body.release()
	}
	return count, err
}

func (body *profileInflightBody) Close() error {
	err := body.ReadCloser.Close()
	body.release()
	return err
}
