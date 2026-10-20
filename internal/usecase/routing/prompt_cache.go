package routing

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	promptCacheBindingLimit = 2048
	promptCacheRetention    = 6 * time.Hour
)

type promptCacheBinding struct {
	accountID         string
	boundAt           time.Time
	cachedInputTokens uint64
}

func promptCacheKeyFromBody(body []byte) string {
	if len(body) == 0 || !utf8.Valid(body) {
		return ""
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	value, _ := object["prompt_cache_key"].(string)
	return strings.TrimSpace(value)
}

func (router *Router) promptCacheOwner(key string, now time.Time) string {
	now = time.Unix(now.Unix(), 0)
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	router.promptCacheMu.Lock()
	defer router.promptCacheMu.Unlock()
	router.prunePromptCacheBindingsLocked(now)
	return router.promptCacheBindings[key].accountID
}

func (router *Router) rememberPromptCacheOwner(accountID, key string, now time.Time) {
	now = time.Unix(now.Unix(), 0)
	accountID, key = strings.TrimSpace(accountID), strings.TrimSpace(key)
	if accountID == "" || key == "" {
		return
	}
	router.promptCacheMu.Lock()
	defer router.promptCacheMu.Unlock()
	if router.promptCacheBindings == nil {
		router.promptCacheBindings = make(map[string]promptCacheBinding)
	}
	router.prunePromptCacheBindingsLocked(now)
	binding, exists := router.promptCacheBindings[key]
	switch {
	case exists && binding.accountID == accountID:
		if binding.boundAt.Before(now) {
			binding.boundAt = now
		}
	case exists && binding.cachedInputTokens > 0:
		if binding.boundAt.Before(now) {
			binding.boundAt = now
		}
	case exists:
		binding.accountID, binding.boundAt = accountID, now
	default:
		binding = promptCacheBinding{accountID: accountID, boundAt: now}
	}
	router.promptCacheBindings[key] = binding
	router.prunePromptCacheBindingsLocked(now)
}

func (router *Router) observePromptCacheHit(accountID, key string, cachedInputTokens uint64, now time.Time) {
	now = time.Unix(now.Unix(), 0)
	accountID, key = strings.TrimSpace(accountID), strings.TrimSpace(key)
	if accountID == "" || key == "" || cachedInputTokens == 0 {
		return
	}
	router.promptCacheMu.Lock()
	defer router.promptCacheMu.Unlock()
	if router.promptCacheBindings == nil {
		router.promptCacheBindings = make(map[string]promptCacheBinding)
	}
	router.prunePromptCacheBindingsLocked(now)
	binding, exists := router.promptCacheBindings[key]
	switch {
	case exists && binding.accountID == accountID:
		if binding.boundAt.Before(now) {
			binding.boundAt = now
		}
		if cachedInputTokens > binding.cachedInputTokens {
			binding.cachedInputTokens = cachedInputTokens
		}
	case exists && cachedInputTokens > binding.cachedInputTokens:
		binding = promptCacheBinding{accountID: accountID, boundAt: now, cachedInputTokens: cachedInputTokens}
	case exists:
		if binding.boundAt.Before(now) {
			binding.boundAt = now
		}
	default:
		binding = promptCacheBinding{accountID: accountID, boundAt: now, cachedInputTokens: cachedInputTokens}
	}
	router.promptCacheBindings[key] = binding
	router.prunePromptCacheBindingsLocked(now)
}

// Prodex records bound_at in Unix seconds; subsecond clocks must not
// expire a binding early or reorder equal-epoch eviction candidates.
func (router *Router) prunePromptCacheBindingsLocked(now time.Time) {
	oldestAllowed := time.Unix(now.Unix(), 0).Add(-promptCacheRetention)
	for key, binding := range router.promptCacheBindings {
		if binding.boundAt.Before(oldestAllowed) {
			delete(router.promptCacheBindings, key)
		}
	}
	for len(router.promptCacheBindings) > promptCacheBindingLimit {
		oldestKey := ""
		var oldest time.Time
		for key, binding := range router.promptCacheBindings {
			if oldestKey == "" || binding.boundAt.Before(oldest) || (binding.boundAt.Equal(oldest) && key < oldestKey) {
				oldestKey, oldest = key, binding.boundAt
			}
		}
		delete(router.promptCacheBindings, oldestKey)
	}
}

func promptCacheCachedInputTokens(value any) uint64 {
	usage, valid := scanNativeUsage(value)
	if !valid {
		return 0
	}
	return usage.CachedInputTokens
}

func (router *Router) observePromptCachePayload(accountID, key string, payload []byte) {
	if key == "" || len(payload) == 0 {
		return
	}
	if value, valid := decodeNativeUsageJSON(payload); valid {
		if cached := promptCacheCachedInputTokens(value); cached > 0 {
			router.observePromptCacheHit(accountID, key, cached, router.now())
		}
	}
}

func (router *Router) wrapPromptCacheSSEObservation(accountID, key string, response *proxymodel.Response) {
	if key == "" || response == nil || response.Body == nil {
		return
	}
	response.Body = &promptCacheObservationBody{
		ReadCloser: response.Body, router: router, accountID: accountID, key: key,
		decoder: sse.NewDecoder(1 << 20),
	}
}

type promptCacheObservationBody struct {
	io.ReadCloser
	router    *Router
	accountID string
	key       string
	decoder   *sse.Decoder
	once      sync.Once
}

func (body *promptCacheObservationBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if count > 0 {
		for _, event := range body.decoder.Feed(buffer[:count]) {
			body.router.observePromptCachePayload(body.accountID, body.key, event)
		}
	}
	if err == io.EOF {
		body.once.Do(func() {
			for _, event := range body.decoder.Finish() {
				body.router.observePromptCachePayload(body.accountID, body.key, event)
			}
		})
	}
	return count, err
}
