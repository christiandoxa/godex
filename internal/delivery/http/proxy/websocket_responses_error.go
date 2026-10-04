package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func responsesWebSocketRouteError(err error) (int, string, string) {
	status := http.StatusServiceUnavailable
	code := "service_unavailable"
	message := "upstream request failed"
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) {
		return status, code, message
	}
	if routeErr.StatusCode > 0 {
		status = routeErr.StatusCode
	}
	if routeErr.Message != "" {
		message = routeErr.Message
	}
	switch {
	case status == http.StatusConflict:
		code = "stale_continuation"
	case status < http.StatusInternalServerError:
		code = "invalid_request_error"
	}
	return status, code, message
}

func responsesWebSocketErrorPayload(status int, code, message string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	return payload
}

func (proxy *Proxy) forwardResponsesWebSocketHTTPBody(
	response *proxymodel.Response,
	prefix []byte,
	writer *websocketFrameWriter,
) error {
	readers := []io.Reader{bytes.NewReader(prefix)}
	if response != nil && response.Body != nil {
		readers = append(readers, response.Body)
	}
	body, err := io.ReadAll(io.LimitReader(io.MultiReader(readers...), proxy.maxInspect+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > proxy.maxInspect {
		return errors.New("websocket error response exceeded safe forwarding limit")
	}
	if len(body) == 0 {
		return nil
	}
	return writer.writeText(body)
}
