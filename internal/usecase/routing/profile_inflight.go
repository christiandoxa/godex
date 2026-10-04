package routing

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
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
	if !hardAffinity && current+weight > effectiveProfileInflightHardLimit(router.profileInflightHardLimit, weight) {
		router.mu.Unlock()
		return nil, false
	}
	router.inflight[accountID] = current + weight
	router.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			if router.inflight[accountID] <= weight {
				delete(router.inflight, accountID)
			} else {
				router.inflight[accountID] -= weight
			}
			close(router.inflightChanged)
			router.inflightChanged = make(chan struct{})
			router.mu.Unlock()
		})
	}, true
}

func (router *Router) waitForProfileInflight(ctx context.Context) error {
	router.mu.Lock()
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
	response, err := router.executeRouted(ctx, request, account, hardAffinity)
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
		if err := router.waitForProfileInflight(ctx); err != nil {
			return nil, err
		}
	}
}

type profileInflightBody struct {
	io.ReadCloser
	release func()
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
