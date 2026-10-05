package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (proxy *Proxy) forwardWebSocketHTTPError(response *proxymodel.Response, prefix []byte, writer *websocketFrameWriter) error {
	readers := []io.Reader{bytes.NewReader(prefix)}
	if response.Body != nil {
		readers = append(readers, response.Body)
	}
	body, err := io.ReadAll(io.LimitReader(io.MultiReader(readers...), proxy.maxRequest+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > proxy.maxRequest {
		return errors.New("websocket error response exceeded safe forwarding limit")
	}
	if len(body) == 0 {
		return nil
	}
	return writer.writeText(body)
}

func websocketRouteError(err error) (int, string, string) {
	status, code, message := http.StatusServiceUnavailable, "service_unavailable", "upstream request failed"
	var routeError *proxymodel.Error
	if errors.As(err, &routeError) {
		if routeError.StatusCode > 0 {
			status = routeError.StatusCode
		}
		message = routeError.Message
		if status == http.StatusConflict {
			code = "stale_continuation"
		} else if status < http.StatusInternalServerError {
			code = "invalid_request_error"
		}
	}
	return status, code, message
}

func websocketErrorPayload(status int, code, message string) []byte {
	payload, _ := json.Marshal(struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Type: "error", Status: status, Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
	return payload
}
