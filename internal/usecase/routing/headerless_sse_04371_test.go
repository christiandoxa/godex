package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const overloadEvent04371 = "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\"}}}\n\n"

func TestProdex04371InspectRequestedSSEWithoutResponseMIME(t *testing.T) {
	fixtures := []struct {
		name            string
		requestedStream bool
		contentType     string
		encoding        string
		wantRetry       bool
		wantPrefix      bool
	}{
		{"requested_missing_mime", true, "", "", true, true},
		{"requested_empty_mime", true, " ", "", true, true},
		{"explicit_sse", false, "text/event-stream", "", true, true},
		{"unary_missing_mime", false, "", "", false, false},
		{"explicit_json", true, "application/json", "", false, false},
		{"explicit_plain", true, "text/plain", "", false, false},
		{"encoded_response", true, "", "gzip", false, false},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			router := &Router{now: time.Now}
			response := &proxymodel.Response{
				StatusCode:         http.StatusOK,
				Header:             make(http.Header),
				Body:               io.NopCloser(strings.NewReader(overloadEvent04371)),
				RequestedStreaming: fixture.requestedStream,
			}
			if fixture.contentType != "" {
				response.Header.Set("Content-Type", fixture.contentType)
			}
			if fixture.encoding != "" {
				response.Header.Set("Content-Encoding", fixture.encoding)
			}
			outcome, pending, err := router.classify(response, "openai")
			if err != nil {
				t.Fatal(err)
			}
			defer pending.close()
			if got := outcome.kind == responseRetry; got != fixture.wantRetry {
				t.Fatalf("unexpected retry=%t want=%t outcome=%+v", got, fixture.wantRetry, outcome)
			}
			if got := len(pending.prefix) > 0; got != fixture.wantPrefix {
				t.Fatalf("inspected prefix=%t want=%t", got, fixture.wantPrefix)
			}
			if fixture.wantRetry && (!outcome.transient || !strings.Contains(string(pending.prefix), "server_is_overloaded")) {
				t.Fatalf("lost overload signal in headerless streamed response: %+v", outcome)
			}
			if response.Header.Get("Content-Type") != fixture.contentType {
				t.Fatalf("response MIME was invented or rewritten: %#v", response.Header)
			}
		})
	}
}

type headerlessSSERequestGateway struct{ stream bool }

func (g *headerlessSSERequestGateway) Execute(_ context.Context, req proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	g.stream = requestedResponsesStream(req)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(overloadEvent04371)),
	}, nil
}

func TestProdex04371OriginalResponsesStreamFlagReachesTransportClassifier(t *testing.T) {
	for _, fixture := range []struct {
		name, requestBody string
		want              bool
	}{
		{"requested_stream", `{"stream":true,"input":"hello"}`, true},
		{"explicit_unary", `{"stream":false,"input":"hello"}`, false},
		{"absent_stream", `{"input":"hello"}`, false},
		{"non_boolean_stream", `{"stream":"true"}`, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &headerlessSSERequestGateway{}
			router, err := NewRouter(Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Home: "/synthetic", Enabled: true}}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			req := proxymodel.Request{
				Path:           "/backend-api/codex/responses",
				Header:         make(http.Header),
				Body:           []byte(fixture.requestBody),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			resp, acquired, err := router.tryExecuteWithProfileInflight(t.Context(), req, proxymodel.Account{
				ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Enabled: true,
			}, false)
			if err != nil || !acquired {
				t.Fatalf("dispatch=%v acquired=%t", err, acquired)
			}
			defer resp.Body.Close()
			if gateway.stream != fixture.want || resp.RequestedStreaming != fixture.want {
				t.Fatalf("original stream request lost: gateway=%t response=%t want=%t", gateway.stream, resp.RequestedStreaming, fixture.want)
			}
		})
	}
}

// A fresh Responses stream without Content-Type must rotate after a
// precommit capacity error rather than committing the synthetic 200
// failure as a unary success. The winning stream keeps its header absence.
type headerlessRotation04371Gateway struct{ owners []string }

func (g *headerlessRotation04371Gateway) Execute(_ context.Context, req proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	g.owners = append(g.owners, account.ID)
	payload := overloadEvent04371
	if account.ID == "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		payload = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered-after-rotation\"}\n\n"
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(payload)),
	}, nil
}
func TestProdex04371FreshHeaderlessSSEPrecommitRotatesBeforeOutput(t *testing.T) {
	gateway := &headerlessRotation04371Gateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Home: "/first", Enabled: true},
				{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Home: "/second", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := proxymodel.Request{
		Path: "/backend-api/codex/responses", Header: make(http.Header),
		Body:           []byte(`{"stream":true,"input":[]}`),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	result, err := router.Forward(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	body, err := io.ReadAll(result.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response := string(result.Result.Prefix) + string(body)
	if strings.Contains(response, "server_is_overloaded") ||
		!strings.Contains(response, "recovered-after-rotation") {
		t.Fatalf("headerless SSE precommit rotation failed: %q", response)
	}
	if strings.Join(gateway.owners, ",") != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("fresh headerless stream chose wrong providers: %v", gateway.owners)
	}
	if result.Result.Response.Header.Get("Content-Type") != "" {
		t.Fatal("missing upstream Content-Type was invented")
	}
}
