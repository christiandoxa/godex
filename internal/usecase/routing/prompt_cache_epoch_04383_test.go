package routing

import (
	"strconv"
	"testing"
	"time"
)

// Prodex 0.438.3's prompt cache state stores Local::now().timestamp()
// (integer seconds). At the exact 6-hour epoch boundary a binding is still
// valid, including when Go observes a fractional time in that second.
func TestProdex04383PromptCacheRetentionRoundsToEpochSeconds(t *testing.T) {
	base := time.Unix(1760000000, 0)
	key := "synthetic-cache"
	owner := "synthetic-main"
	for _, tc := range []struct {
		name string
		now  time.Time
		want string
	}{
		{"before-deadline", base.Add(promptCacheRetention - time.Second + 999*time.Millisecond), owner},
		{"inclusive-exact-deadline", base.Add(promptCacheRetention), owner},
		{"fractional-deadline", base.Add(promptCacheRetention + 999*time.Millisecond), owner},
		{"expired-next-second", base.Add(promptCacheRetention + time.Second), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := &Router{promptCacheBindings: map[string]promptCacheBinding{
				key: {accountID: owner, boundAt: base},
			}}
			if got := router.promptCacheOwner(key, tc.now); got != tc.want {
				t.Fatalf("owner at epoch %d+%dms=%q want=%q", tc.now.Unix(), tc.now.Nanosecond()/1e6, got, tc.want)
			}
		})
	}
}

// In the compiled reference every observation within a second has the same
// bound_at epoch. Fractional timestamps must not change which tied binding is
// discarded when the bounded prompt-cache affinity map exceeds capacity.
func TestProdex04383PromptCachePruneTieUsesKeyOrderNotSubsecondTime(t *testing.T) {
	base := time.Unix(1760000000, 0)
	router := &Router{promptCacheBindings: make(map[string]promptCacheBinding)}
	router.rememberPromptCacheOwner("owner-a", "a", base.Add(900*time.Millisecond))
	router.rememberPromptCacheOwner("owner-b", "b", base.Add(100*time.Millisecond))
	for i := 0; i < promptCacheBindingLimit-1; i++ {
		router.rememberPromptCacheOwner("other", "fill-"+strconv.Itoa(i), base.Add(time.Second))
	}
	// The cap is exceeded; both 'a' and 'b' have identical oldest epoch.
	if got := router.promptCacheOwner("a", base.Add(2*time.Second)); got != "" {
		t.Fatalf("lexically first tied binding retained: %q", got)
	}
	if got := router.promptCacheOwner("b", base.Add(2*time.Second)); got != "owner-b" {
		t.Fatalf("lexically second tied binding was pruned: %q", got)
	}
}
