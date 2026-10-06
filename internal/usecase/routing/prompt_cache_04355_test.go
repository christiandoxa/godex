package routing

import (
	"io"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04355PromptCacheSSEFlushesFinalUnterminatedUsageEvent(t *testing.T) {
	now := time.Unix(260_000, 0)
	router := &Router{now: func() time.Time { return now }, promptCacheBindings: make(map[string]promptCacheBinding)}
	response := &proxymodel.Response{Body: io.NopCloser(strings.NewReader(`data: {"usage":{"input_tokens_details":{"cached_tokens":7}}}`))}
	router.wrapPromptCacheSSEObservation("account-a", "cache-1", response)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if got := router.promptCacheOwner("cache-1", now); got != "account-a" {
		t.Fatalf("final SSE prompt-cache owner = %q, want account-a", got)
	}
}
