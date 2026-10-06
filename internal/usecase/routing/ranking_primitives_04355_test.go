package routing

import (
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04355BackoffSortKeyMatchesMojoLiterals(t *testing.T) {
	now := time.Unix(10, 0)
	for _, test := range []struct {
		name                      string
		circuit, transport, retry int64
		want                      candidateBackoffSortKey
	}{
		{"none", 0, 0, 0, candidateBackoffSortKey{}},
		{"circuit", 40, 0, 0, candidateBackoffSortKey{class: 1, first: 40}},
		{"transport", 0, 20, 0, candidateBackoffSortKey{class: 2, first: 20}},
		{"retry", 0, 0, 30, candidateBackoffSortKey{class: 3, first: 30}},
		{"circuit transport", 40, 20, 0, candidateBackoffSortKey{class: 4, first: 20, second: 40}},
		{"circuit retry", 40, 0, 30, candidateBackoffSortKey{class: 5, first: 30, second: 40}},
		{"transport retry", 0, 20, 30, candidateBackoffSortKey{class: 6, first: 20, second: 30}},
		{"all", 40, 20, 30, candidateBackoffSortKey{class: 7, first: 20, second: 40, retry: 30}},
		{"all retry earliest", 40, 20, 12, candidateBackoffSortKey{class: 7, first: 20, second: 40, retry: 12}},
		{"all retry latest", 40, 20, 80, candidateBackoffSortKey{class: 7, first: 20, second: 40, retry: 80}},
	} {
		t.Run(test.name, func(t *testing.T) {
			at := func(unix int64) time.Time {
				if unix == 0 {
					return time.Time{}
				}
				return time.Unix(unix, 0)
			}
			got := profileBackoffSortKey(at(test.circuit), at(test.transport), at(test.retry), now)
			if got != test.want {
				t.Fatalf("backoff key = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestProdex04355PromptCacheAffinityHashMatchesMojoLiterals(t *testing.T) {
	for _, test := range []struct {
		key, profile string
		want         uint64
	}{
		{"cache-1", "account-a", 5682685861876336996},
		{"cache-1", "account-b", 5682684762364708785},
		{"  shared key  ", "alpha", 17157215515782083699},
	} {
		priority, got := promptCacheAffinitySortKey(test.key, "", test.profile)
		if priority != 0 || got != test.want {
			t.Fatalf("prompt affinity %q/%q = (%d,%d), want (0,%d)", test.key, test.profile, priority, got, test.want)
		}
	}
	priority, score := promptCacheAffinitySortKey("cache-1", "account-b", "account-b")
	if priority != 0 || score != 0 {
		t.Fatalf("owner affinity = (%d,%d), want (0,0)", priority, score)
	}
	priority, _ = promptCacheAffinitySortKey("cache-1", "account-b", "account-a")
	if priority != 1 {
		t.Fatalf("non-owner priority = %d, want 1", priority)
	}
}

func TestProdex04355SelectionJitterMatchesRustDefaultHasherLiterals(t *testing.T) {
	for _, test := range []struct {
		sequence uint64
		profile  string
		route    quotamodel.RouteKind
		want     uint64
	}{
		{0, "alpha", quotamodel.RouteKindResponses, 7738385989630893064},
		{0, "alpha", quotamodel.RouteKindWebSocket, 1563637110101824712},
		{1, "beta", quotamodel.RouteKindResponses, 5097511859099357445},
		{42, "alpha", quotamodel.RouteKindResponses, 13720159197250930926},
		{42, "beta", quotamodel.RouteKindStandard, 11060376130333464164},
	} {
		if got := selectionJitter(test.sequence, test.profile, test.route); got != test.want {
			t.Fatalf("jitter %d/%s/%s = %d, want %d", test.sequence, test.profile, routeHealthRoute(test.route), got, test.want)
		}
	}
}

func TestProdex04355PromptCacheHitOwnershipUsesHighestCachedTokens(t *testing.T) {
	now := time.Unix(300_000, 0)
	router := &Router{promptCacheBindings: make(map[string]promptCacheBinding)}
	router.rememberPromptCacheOwner("account-a", "cache", now)
	router.observePromptCacheHit("account-a", "cache", 10, now.Add(time.Second))
	router.rememberPromptCacheOwner("account-b", "cache", now.Add(2*time.Second))
	if got := router.promptCacheOwner("cache", now.Add(2*time.Second)); got != "account-a" {
		t.Fatalf("simple remember replaced hit owner with %q", got)
	}
	router.observePromptCacheHit("account-b", "cache", 9, now.Add(3*time.Second))
	if got := router.promptCacheOwner("cache", now.Add(3*time.Second)); got != "account-a" {
		t.Fatalf("weaker cache hit replaced owner with %q", got)
	}
	router.observePromptCacheHit("account-b", "cache", 11, now.Add(4*time.Second))
	if got := router.promptCacheOwner("cache", now.Add(4*time.Second)); got != "account-b" {
		t.Fatalf("stronger cache hit owner = %q, want account-b", got)
	}
}
