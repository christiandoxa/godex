package proxy

import (
	"io"
	"net/http"
)

type Request struct {
	RequestID                       uint64
	Method, Path, RawPath, RawQuery string
	Header                          http.Header
	Body                            []byte
	FirstEventRetryUsed             bool
}
type Response struct {
	StatusCode          int
	Header              http.Header
	Body                io.ReadCloser
	Trailer             http.Header
	FirstEventRetryUsed bool
	FirstEventCommitted bool
	PrecommitFailure    *PrecommitFailure
}
type PrecommitFailure struct {
	Code      string
	Transport bool
}
type Forwarded struct {
	Response  *Response
	Prefix    []byte
	AccountID string
	Failed    bool
}
type Auth struct {
	AccessToken string `json:"-"`
	AccountID   string `json:"-"`
}
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string { return e.Message }
