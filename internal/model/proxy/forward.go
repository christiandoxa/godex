package proxy

import (
	"io"
	"net/http"
	"time"
)

type WebSocketPolicy struct {
	PromoteCommittedProfile bool
	RequestPreviousResponse bool
	RequestSession          bool
	RequestTurnState        bool
	TurnStateOverride       bool
}

type Request struct {
	RequestID                       uint64
	Method, Path, RawPath, RawQuery string
	Header                          http.Header
	Body                            []byte
	WebSocketMessage                bool
	WebSocketSessionID              uint64
	WebSocketPolicy                 WebSocketPolicy
	FirstEventRetryUsed             bool
}
type Response struct {
	StatusCode             int
	Header                 http.Header
	Body                   io.ReadCloser
	Trailer                http.Header
	WebSocketResponseID    string
	WebSocketTurnState     string
	WebSocketFrames        bool
	WebSocketReusedSession bool
	WebSocketReuseIdle     time.Duration
	FirstEventRetryUsed    bool
	FirstEventCommitted    bool
	PrecommitFailure       *PrecommitFailure
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
