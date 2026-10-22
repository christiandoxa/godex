package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type geminiInvalidGateway04380 struct{ calls int }

func (g *geminiInvalidGateway04380) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	g.calls++
	return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest,
		Message: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"}
}

// Rejected local tools are a deterministic 400 from the request, NOT a
// failed provider profile. An ordinary retry or profile rotation can never
// fix these bytes and must not turn it into a misleading 503 timeout.
func TestProdex04380LocalGeminiToolValidationBypassesRouterRetryAndRotation(t *testing.T) {
	gateway := &geminiInvalidGateway04380{}
	waits := 0
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "synthetic-one", Home: "/synthetic/one", Enabled: true},
				{ID: "synthetic-two", Home: "/synthetic/two", Enabled: true},
			}, nil
		},
		Wait: func(context.Context, time.Duration) error {
			waits++
			return errors.New("retry wait should never happen")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	_, err = router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/godex/responses", Header: make(http.Header),
	})
	var requestErr *proxymodel.Error
	if !errors.As(err, &requestErr) || requestErr.StatusCode != http.StatusBadRequest ||
		!strings.Contains(requestErr.Message, "tools[0].function.parameters") {
		t.Fatalf("malformed Gemini tool request not surfaced as HTTP 400: %v", err)
	}
	if gateway.calls != 1 || waits != 0 {
		t.Fatalf("malformed tool request retried or rotated profile: calls=%d waits=%d", gateway.calls, waits)
	}
}

func TestProdex04380LocalGeminiToolValidationErrorMatchIsFailClosed(t *testing.T) {
	cases := []struct {
		err   error
		valid bool
	}{
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a non-empty string"}, valid: true},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` is required"}, valid: true},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"}, valid: true},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1"}, valid: true},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict"}, valid: true},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_candidate_count: unrelated upstream error"}},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_candidate_count: Gemini request field `other` must be omitted, null, or 1"}},
		{err: &proxymodel.Error{StatusCode: 502, Message: "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"}},
		{err: &proxymodel.Error{StatusCode: 400, Message: "ordinary malformed upstream input"}},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_tool_declaration: unrelated adapter"}},
		{err: &proxymodel.Error{StatusCode: 502, Message: "invalid_tool_declaration: Gemini request field `tools[0].function.name` must be a string"}},
		{err: &proxymodel.Error{StatusCode: 400, Message: "invalid_tool_declaration: Gemini request field `other` not a tool"}},
		{err: errors.New("invalid_tool_declaration: Gemini request field `tools[0]` must be an object")},
	}
	for index, tc := range cases {
		if got := isProxyPreparationError(tc.err); got != tc.valid {
			t.Fatalf("case %d local validation classification=%t want=%t", index, got, tc.valid)
		}
	}
}
