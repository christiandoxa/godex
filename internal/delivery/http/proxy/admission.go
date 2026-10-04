package proxy

import (
	"net/http"
)

const defaultActiveRequestLimit = 64

type activeRequestHandler struct {
	next  http.Handler
	slots chan struct{}
}

func newActiveRequestHandler(next http.Handler, limit int) http.Handler {
	if limit <= 0 {
		limit = defaultActiveRequestLimit
	}
	return &activeRequestHandler{next: next, slots: make(chan struct{}, limit)}
}

func (handler *activeRequestHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	select {
	case handler.slots <- struct{}{}:
		defer func() { <-handler.slots }()
	case <-request.Context().Done():
		return
	}
	handler.next.ServeHTTP(writer, request)
}
