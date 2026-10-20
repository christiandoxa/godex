package proxy

import (
	"io"
	"net/http"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type WebSocketPolicy struct {
	PromoteCommittedProfile bool
	RequestPreviousResponse bool
	RequestSession          bool
	RequestTurnState        bool
	TurnStateOverride       bool
	RealtimeDuplex          bool
}

type Request struct {
	RequestID                       uint64
	SelectionSequence               uint64
	Method, Path, RawPath, RawQuery string
	Header                          http.Header
	Body                            []byte
	QuotaSelection                  quotamodel.Selection
	WebSocketMessage                bool
	WebSocketSessionID              uint64
	WebSocketPolicy                 WebSocketPolicy
	FirstEventRetryUsed             bool
}
type Response struct {
	// Auth evidence is internal-only: never serialize a fingerprint or a
	// request timestamp to clients, persisted conversations or proxy logs.
	AuthCredentialDigest    [32]byte  `json:"-"`
	AuthCredentialKnown     bool      `json:"-"`
	AuthRequestStartedAt    time.Time `json:"-"`
	StatusCode              int
	Header                  http.Header
	Body                    io.ReadCloser
	Trailer                 http.Header
	WebSocketResponseID     string
	WebSocketEvent          *WebSocketEvent `json:"-"`
	WebSocketTurnState      string
	WebSocketFrames         bool
	WebSocketReusedSession  bool
	WebSocketReuseIdle      time.Duration
	UpstreamConnectDuration time.Duration
	WebSocketRealtimeDuplex bool
	FirstEventRetryUsed     bool
	FirstEventCommitted     bool
	// RequestedStreaming is internal routing evidence from the original
	// Responses request. It prevents a missing upstream Content-Type from
	// bypassing the precommit SSE inspector. Never serialize this field.
	RequestedStreaming bool `json:"-"`
	// RequestPreviousResponseID carries a verified, source-owned Responses
	// request identifier only within the committed HTTP stream observer.
	// It is never forwarded or serialized to clients.
	RequestPreviousResponseID string `json:"-"`
	PrecommitFailure          *PrecommitFailure
}

// WebSocketEvent is borrowed parser output, valid until the next body Read.
// The body reader and its delivery observer consume it on the same goroutine.
type WebSocketEvent struct {
	Sequence uint64
	Payload  []byte
}
type PrecommitFailure struct {
	Code string
	// RetryAdviceJSON contains at most 64 KiB of precommit WebSocket error
	// metadata for routing; it is never persisted or exposed to clients.
	RetryAdviceJSON           []byte `json:"-"`
	Transport                 bool
	InvalidPreviousResponseID bool
	StaleContinuation         bool
}
type Forwarded struct {
	Response     *Response
	Prefix       []byte
	AccountID    string
	ProviderKind string
	Failed       bool
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
