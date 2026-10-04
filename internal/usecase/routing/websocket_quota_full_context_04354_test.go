package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const (
	websocketQuotaOwnerA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	websocketQuotaOwnerB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestProdex04354WebSocketQuotaSignalsFullContextWhenFallbackReady(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-main"),
		websocketQuotaFailureResponse(),
		websocketCommittedTestResponse("resp-second"),
	}}
	redeemer := &fakeRoutingRedeemer{accountID: websocketQuotaOwnerA, redeemed: true}
	store := routingrepo.NewStore(t.TempDir())
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA, Bindings: store,
		AutoRedeem: true, Redeemer: redeemer,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn one\"}]}",
		101,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	signal, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"previous_response_id\":\"resp-main\",\"input\":[{\"type\":\"custom_tool_call_output\",\"call_id\":\"call-1\",\"output\":\"done\"}]}",
		101,
	))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(signal.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := signal.Close(); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("signal JSON: %v body=%s", err, body)
	}
	errorValue, _ := value["error"].(map[string]any)
	if signal.Result.AccountID != websocketQuotaOwnerA ||
		signal.Result.Response.StatusCode != http.StatusBadRequest ||
		websocketJSONText(errorValue["code"]) != "previous_response_not_found" ||
		websocketJSONText(errorValue["message"]) != websocketQuotaFullContextMessage ||
		strings.Contains(string(body), "insufficient_quota") ||
		redeemer.calls != 0 {
		t.Fatalf("quota signal = account=%q status=%d body=%s redeemer=%d",
			signal.Result.AccountID, signal.Result.Response.StatusCode, body, redeemer.calls)
	}

	previousOwner, err := router.affinity.owner(
		t.Context(), affinityKeys{previous: "resp-main"}, router.now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionOwner, err := router.affinity.owner(
		t.Context(), affinityKeys{session: "session-quota"}, router.now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if previousOwner != "" || sessionOwner != "" {
		t.Fatalf("quota signal retained affinity: previous=%q session=%q", previousOwner, sessionOwner)
	}
	persisted, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range persisted {
		if binding.Kind == "previous" || binding.Kind == "session" {
			t.Fatalf("released quota affinity persisted: %#v", binding)
		}
	}

	replay, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-quota\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn one\"},{\"type\":\"message\",\"role\":\"assistant\",\"content\":\"turn one result\"},{\"type\":\"message\",\"role\":\"user\",\"content\":\"turn two\"}]}",
		102,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if replay.Result.AccountID != websocketQuotaOwnerB ||
		replay.Result.Response.WebSocketResponseID != "resp-second" ||
		strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerA+","+websocketQuotaOwnerB {
		t.Fatalf("full replay = account=%q response=%q attempts=%v",
			replay.Result.AccountID, replay.Result.Response.WebSocketResponseID, gateway.accounts)
	}
}

func TestProdex04354WebSocketSessionOnlyQuotaRotatesSameRequest(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-first"),
		websocketQuotaFailureResponse(),
		websocketCommittedTestResponse("resp-second"),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return websocketQuotaAccounts(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-soft\"}",
		103,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	next, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-soft\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":\"next\"}]}",
		103,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.Result.AccountID != websocketQuotaOwnerB ||
		strings.Join(gateway.accounts, ",") != websocketQuotaOwnerA+","+websocketQuotaOwnerA+","+websocketQuotaOwnerB {
		t.Fatalf("session quota fallback = account=%q attempts=%v", next.Result.AccountID, gateway.accounts)
	}
}

func TestProdex04354WebSocketQuotaKeepsAffinityWhenNoFallbackReady(t *testing.T) {
	gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
		websocketCommittedTestResponse("resp-main"),
		websocketQuotaFailureResponse(),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: websocketQuotaOwnerA, Home: "/a", Enabled: true},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-only\"}",
		104,
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	quota, err := router.Forward(t.Context(), websocketDispatchRequest(
		"{\"type\":\"response.create\",\"session_id\":\"session-only\",\"previous_response_id\":\"resp-main\",\"input\":[{\"type\":\"custom_tool_call_output\",\"call_id\":\"call-1\",\"output\":\"done\"}]}",
		104,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer quota.Close()
	if quota.Result.Response.PrecommitFailure == nil ||
		quota.Result.Response.PrecommitFailure.Code != "insufficient_quota" {
		t.Fatalf("quota failure was replaced without fallback: %#v", quota.Result.Response)
	}
	owner, err := router.affinity.owner(
		t.Context(), affinityKeys{session: "session-only"}, router.now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if owner != websocketQuotaOwnerA {
		t.Fatalf("quota without fallback released owner: %q", owner)
	}
}

func websocketQuotaAccounts() []proxymodel.Account {
	return []proxymodel.Account{
		{ID: websocketQuotaOwnerA, Home: "/a", Enabled: true},
		{ID: websocketQuotaOwnerB, Home: "/b", Enabled: true},
	}
}

func websocketQuotaFailureResponse() *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode:      http.StatusOK,
		Header:          make(http.Header),
		Body:            io.NopCloser(strings.NewReader("quota-frame")),
		WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code: "insufficient_quota",
		},
	}
}
