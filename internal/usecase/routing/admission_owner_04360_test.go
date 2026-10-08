package routing

import (
	"context"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04360AdmissionOwnerMustBeVerifiedAndStillRegistered(t *testing.T) {
	accounts := []proxymodel.Account{{ID: "owner-a", Enabled: true, Home: "/a"}, {ID: "owner-b", Enabled: true, Home: "/b"}}
	router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = router.affinity.rememberVerified(context.Background(), "owner-a",
		affinityKeys{previous: "known-response", turn: "known-turn", session: "known-session"}, now); err != nil {
		t.Fatal(err)
	}
	if err = router.affinity.rememberVerified(context.Background(), "owner-b",
		affinityKeys{turn: "conflict-turn"}, now); err != nil {
		t.Fatal(err)
	}
	if err = router.affinity.rememberVerified(context.Background(), "owner-a",
		affinityKeys{session: "__compact_session__:alias-session"}, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		lane    quotamodel.RouteKind
		headers http.Header
		body    string
		want    bool
	}{
		{"verified responses response", quotamodel.RouteKindResponses, nil, `{"previous_response_id":"known-response"}`, true},
		{"verified response turn", quotamodel.RouteKindResponses, http.Header{"X-Codex-Turn-State": {"known-turn"}}, "{}", true},
		{"verified compact session", quotamodel.RouteKindCompact, nil, `{"session_id":"known-session"}`, true},
		{"verified compact turn", quotamodel.RouteKindCompact, http.Header{"X-Codex-Turn-State": {"known-turn"}}, "{}", true},
		{"verified compact lineage alias", quotamodel.RouteKindCompact, nil, `{"session_id":"alias-session"}`, true},
		{"unverified response", quotamodel.RouteKindResponses, nil, `{"previous_response_id":"fake-response"}`, false},
		{"unverified compact", quotamodel.RouteKindCompact, nil, `{"session_id":"fake-session"}`, false},
		{"ordinary new compact", quotamodel.RouteKindCompact, nil, `{"model":"gpt-test"}`, false},
		{"session alone not response priority", quotamodel.RouteKindResponses, nil, `{"session_id":"known-session"}`, false},
		{"standard cannot bypass", quotamodel.RouteKindStandard, nil, `{"previous_response_id":"known-response"}`, false},
		{"malformed JSON", quotamodel.RouteKindCompact, nil, `{"session_id":`, false},
		{"conflicting binding and session", quotamodel.RouteKindCompact, http.Header{"X-Codex-Turn-State": {"conflict-turn"}}, `{"session_id":"known-session"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := router.HasVerifiedAdmissionOwner(context.Background(), tc.lane, tc.headers, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("verified owner for %q (%s) = %t want %t", tc.name, tc.body, got, tc.want)
			}
		})
	}
	if !router.HasVerifiedCompactPressureOwner(context.Background(), nil, []byte(`{"previous_response_id":"known-response"}`)) {
		t.Fatal("existing previous response owner was shed from compact pressure continuation")
	}
	if !router.HasVerifiedCompactPressureOwner(context.Background(), nil, []byte(`{"session_id":"alias-session"}`)) {
		t.Fatal("existing compact lineage alias was shed")
	}
	if router.HasVerifiedCompactPressureOwner(context.Background(), http.Header{"X-Codex-Turn-State": {"conflict-turn"}}, []byte(`{"session_id":"known-session"}`)) {
		t.Fatal("conflicting direct session owner bypassed compact pressure")
	}
	accounts = []proxymodel.Account{{ID: "owner-b", Enabled: true, Home: "/b"}}
	if router.HasVerifiedAdmissionOwner(context.Background(), quotamodel.RouteKindCompact, nil, []byte(`{"session_id":"known-session"}`)) {
		t.Fatal("deleted owner gained admission priority")
	}
}
