package routing

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type prodex04359CopilotIDGateway struct{ payload []byte }

func (g *prodex04359CopilotIDGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(string(g.payload))),
	}, nil
}

func TestProdex04359CopilotResponseIDShapesKeepContinuationAffinity(t *testing.T) {
	for _, fixture := range []struct {
		name, payload, responseID string
	}{
		{"nested response", `{"response":{"id":"  resp-nested  "}}`, "resp-nested"},
		{"top-level id", `{"id":"resp-top"}`, "resp-top"},
		{"response_id", `{"response_id":"resp-event"}`, "resp-event"},
		{"camel responseId", `{"responseId":"resp-camel"}`, "resp-camel"},
		{"message id", `{"message":{"id":"anthropic-message"}}`, "anthropic-message"},
		{"unicode trim", `{"response":{"id":" 🦀 resp 🦀 "}}`, "🦀 resp 🦀"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			router, err := NewRouter(Config{
				Gateway: &prodex04359CopilotIDGateway{payload: []byte(fixture.payload)},
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{
						{ID: "copilot-a", Enabled: true, Home: "/a", Provider: proxymodel.Provider{Kind: "copilot"}},
					}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := router.Forward(context.Background(), proxymodel.Request{
				Path: "/backend-api/godex/responses", Header: make(http.Header),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: fixture.responseID}, time.Now())
			if err != nil || owner != "copilot-a" {
				t.Fatalf("Copilot payload %s did not bind %q to copilot-a: owner %q, error %v", fixture.payload, fixture.responseID, owner, err)
			}
		})
	}
}

func TestProdex04359NonCopilotEventRootIDNeverClaimsAffinity(t *testing.T) {
	router, err := NewRouter(Config{
		Gateway: &prodex04359CopilotIDGateway{payload: []byte(`{"object":"event","id":"ordinary-event"}`)},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "openai-a", Enabled: true, Home: "/a", Provider: proxymodel.Provider{Kind: "openai"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	owner, err := router.affinity.owner(context.Background(), affinityKeys{previous: "ordinary-event"}, time.Now())
	if err != nil || owner != "" {
		t.Fatalf("ordinary OpenAI event unexpectedly bound: owner %q err %v", owner, err)
	}
}

func TestProdex04359CopilotResponseIDFallbackAndSSEPrecedence(t *testing.T) {
	cases := []struct {
		name, payload string
		stream        bool
		want          []string
	}{
		{"nested blank suppresses root ID", `{"response":{"id":"\u2003"},"id":"top-ignored"}`, false, nil},
		{"root blank suppresses response_id", `{"id":"  ","response_id":"top-ignored"}`, false, nil},
		{"nested nonstring uses root", `{"response":{"id":42},"id":"top-used"}`, false, []string{"top-used"}},
		{"native camel precedes message", `{"responseId":"native-id","message":{"id":"message-id"}}`, false, []string{"native-id"}},
		{"blank native camel suppresses message fallback", `{"responseId":"  ","message":{"id":"ignored"}}`, false, nil},
		{"multiple SSE response IDs", `data: {"responseId":"first"}

data: {"message":{"id":"second"}}

data: {"responseId":"first"}

`, true, []string{"first", "second"}},
		{"invalid JSON", `{"response":{"id":"unfinished"`, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := copilotResponseIDs([]byte(tc.payload), tc.stream)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("IDs for %q = %v; want %v", tc.payload, got, tc.want)
			}
		})
	}
}
