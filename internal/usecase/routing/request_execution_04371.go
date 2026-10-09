package routing

import (
	"context"
	"io"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	release := router.beginRequestInFlight(account.ID, request.QuotaSelection)
	response, err := router.executeAccount(ctx, request, account)
	if response != nil {
		response.RequestedStreaming = requestedResponsesStream(request)
	}
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		release()
		return nil, err
	}
	if response == nil || response.Body == nil {
		release()
		return response, nil
	}
	body := &inFlightBody{ReadCloser: response.Body, release: release}
	if duplex, ok := response.Body.(io.ReadWriteCloser); ok {
		response.Body = &inFlightDuplexBody{inFlightBody: body, duplex: duplex}
	} else {
		response.Body = body
	}
	return response, nil
}
