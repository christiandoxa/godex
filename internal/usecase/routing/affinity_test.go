package routing

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestAffinityHardnessMatchesProdexSessionScope(t *testing.T) {
	for _, test := range []struct {
		name     string
		keys     affinityKeys
		route    quotamodel.RouteKind
		wantHard bool
		wantSoft bool
	}{
		{name: "responses session", keys: affinityKeys{session: "s"}, route: quotamodel.RouteKindResponses, wantSoft: true},
		{name: "websocket session", keys: affinityKeys{session: "s"}, route: quotamodel.RouteKindWebSocket, wantSoft: true},
		{name: "standard session", keys: affinityKeys{session: "s"}, route: quotamodel.RouteKindStandard, wantSoft: true},
		{name: "compact session", keys: affinityKeys{session: "s"}, route: quotamodel.RouteKindCompact, wantHard: true},
		{name: "previous response", keys: affinityKeys{previous: "r"}, route: quotamodel.RouteKindResponses, wantHard: true},
		{name: "turn state", keys: affinityKeys{turn: "t"}, route: quotamodel.RouteKindResponses, wantHard: true},
		{name: "thread", keys: affinityKeys{thread: "t"}, route: quotamodel.RouteKindResponses, wantHard: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := quotamodel.Selection{RouteKind: test.route}
			if got := test.keys.hasHardAffinity(selection); got != test.wantHard {
				t.Fatalf("hard affinity = %t, want %t", got, test.wantHard)
			}
			if got := test.keys.hasSoftSessionAffinity(selection); got != test.wantSoft {
				t.Fatalf("soft session affinity = %t, want %t", got, test.wantSoft)
			}
		})
	}
}

func TestAffinityExpiresAndRejectsConflicts(t *testing.T) {
	store := newAffinityStore()
	now := time.Unix(100, 0)
	keys := affinityKeys{previous: "response-1", turn: "turn-1", session: "session-1"}
	if err := store.remember(context.Background(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if owner, err := store.owner(context.Background(), keys, now.Add(affinityTTL-time.Second)); err != nil || owner != "account-a" {
		t.Fatalf("owner before expiry = %q, %v", owner, err)
	}
	if owner, err := store.owner(context.Background(), keys, now.Add(affinityTTL)); err != nil || owner != "" {
		t.Fatalf("owner at expiry = %q, %v", owner, err)
	}
	if err := store.remember(context.Background(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	if err := store.remember(context.Background(), "account-b", affinityKeys{previous: "response-1"}, now); err == nil {
		t.Fatal("expected affinity conflict")
	}
}

func TestResponseAffinityReadsNestedResponseIdentifiers(t *testing.T) {
	keys := responseObjectAffinity(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         "response-nested",
			"session_id": "session-nested",
			"turn_state": "turn-nested",
		},
	})
	want := affinityKeys{previous: "response-nested", session: "session-nested", turn: "turn-nested"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("nested affinity = %#v, want %#v", keys, want)
	}
}

func TestResponseAffinityMatchesProdexResponseMetadataPrecedence(t *testing.T) {
	got := responseAffinity(http.Header{}, []byte(`{"id":"event-id","response_id":"resp-root","object":"response","headers":{"x-codex-turn-state":"root-state"},"response":{"id":"resp-nested","headers":[["X-CODEX-TURN-STATE",[" nested-state ","later"]]],"turn_state":"response-state","turnState":"camel-state"}}`), false)
	want := affinityKeys{previous: "resp-nested", turn: "nested-state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nested response metadata = %#v, want %#v", got, want)
	}

	got = responseAffinity(http.Header{}, []byte(`{"object":"model.response","id":"resp-object","headers":[{"key":"x-codex-turn-state","values":[" root-state "]}]}`), false)
	want = affinityKeys{previous: "resp-object", turn: "root-state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root response metadata = %#v, want %#v", got, want)
	}

	got = responseAffinity(http.Header{}, []byte(`{"object":"event","id":"event-id"}`), false)
	want = affinityKeys{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non-response root id = %#v, want %#v", got, want)
	}

	got = responseAffinity(http.Header{}, []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-sse\",\"turnState\":\" camel-state \"}}\n\n"), true)
	want = affinityKeys{previous: "resp-sse", turn: "camel-state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SSE response metadata = %#v, want %#v", got, want)
	}
}

func TestResponseTurnStateIsOwnerBoundAndExpires(t *testing.T) {
	store := newAffinityStore()
	now := time.Unix(100, 0)
	store.rememberResponseTurnState("resp_owner", "account-a", "turn-owner", now)
	if got := store.responseTurnState("resp_owner", "account-b", now); got != "" {
		t.Fatalf("turn state for another account = %q", got)
	}
	if got := store.responseTurnState("resp_owner", "account-a", now); got != "turn-owner" {
		t.Fatalf("turn state = %q, want turn-owner", got)
	}
	if got := store.responseTurnState("resp_owner", "account-a", now.Add(affinityTTL)); got != "" {
		t.Fatalf("turn state at expiry = %q", got)
	}
}

func TestResponseTurnStateRejectsOversizedValues(t *testing.T) {
	store := newAffinityStore()
	store.rememberResponseTurnState("resp_owner", "account-a", strings.Repeat("x", maxAffinityValue+1), time.Now())
	if got := store.responseTurnState("resp_owner", "account-a", time.Now()); got != "" {
		t.Fatal("oversized turn state was retained")
	}
}
