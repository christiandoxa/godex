package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type rotatingAuthReader struct {
	calls int
}

func (reader *rotatingAuthReader) ReadAuth(context.Context, string) (proxymodel.Auth, error) {
	reader.calls++
	token := "first-token"
	if reader.calls > 1 {
		token = "second-token"
	}
	return proxymodel.Auth{AccessToken: token, AccountID: "workspace-1"}, nil
}

func TestConsumeResetCreditUsesEndpointHeadersBodyAndAuthReload(t *testing.T) {
	auth := &rotatingAuthReader{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		assertRedeemHTTPRequest(t, request, calls)
		if calls == 1 {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"outcome":"alreadyRedeemed"}`))
	}))
	defer server.Close()

	client, err := NewQuotaClient(server.URL+"/backend-api", nil, auth)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := client.ConsumeResetCredit(context.Background(), "/synthetic/home", "", true, "prodex-manual-redeem-fixture")
	if err != nil || outcome != quotamodel.RedeemAlreadyRedeemed || calls != 2 || auth.calls != 2 {
		t.Fatalf("outcome=%q calls=%d auth-calls=%d err=%v", outcome, calls, auth.calls, err)
	}
}

func assertRedeemHTTPRequest(t *testing.T, request *http.Request, call int) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != "/backend-api/wham/rate-limit-reset-credits/consume" {
		t.Fatalf("request = %s %s", request.Method, request.URL.Path)
	}
	if request.Header.Get("ChatGPT-Account-Id") != "workspace-1" {
		t.Fatalf("account header = %q", request.Header.Get("ChatGPT-Account-Id"))
	}
	var body redeemRequest
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID != "prodex-manual-redeem-fixture" {
		t.Fatalf("request id = %q", body.RequestID)
	}
	wantAuth := "Bearer first-token"
	if call > 1 {
		wantAuth = "Bearer second-token"
	}
	if request.Header.Get("Authorization") != wantAuth {
		t.Fatalf("auth = %q, want %q", request.Header.Get("Authorization"), wantAuth)
	}
}

func TestConsumeResetCreditSupportsNonBackendBaseAndNoProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/codex/rate-limit-reset-credits/consume" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"outcome":"noCredit"}`))
	}))
	defer server.Close()
	client, err := NewQuotaClient("https://chatgpt.com/backend-api", nil, &rotatingAuthReader{})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := client.ConsumeResetCredit(context.Background(), "/synthetic/home", server.URL, true, "prodex-manual-redeem-fixture")
	if err != nil || outcome != quotamodel.RedeemNoCredit {
		t.Fatalf("outcome = %q, err = %v", outcome, err)
	}
	policyClient, err := client.clientForPolicy(true)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := policyClient.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatalf("no-proxy transport = %#v", policyClient.Transport)
	}
}
