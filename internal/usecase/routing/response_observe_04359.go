package routing

import (
	"context"
	"net/http"
	"strings"
)

func (router *Router) Observe(ctx context.Context, accountID string, headers http.Header, body []byte, stream bool, providerKinds ...string) error {
	router.observeTokenUsage(ctx, accountID, body)
	keys := responseAffinity(headers, body, stream)
	var extraIDs []string
	if len(providerKinds) > 0 && strings.EqualFold(strings.TrimSpace(providerKinds[0]), "copilot") {
		// Copilot accepts native responseId and message.id shapes as well as
		// OpenAI response IDs. Generic OpenAI event IDs must remain ignored.
		ids := copilotResponseIDs(body, stream)
		keys.previous = ""
		if len(ids) > 0 {
			keys.previous, extraIDs = ids[0], ids[1:]
		}
	}
	now := router.now()
	if err := router.affinity.rememberVerified(ctx, accountID, keys, now); err != nil {
		return err
	}
	router.affinity.rememberResponseTurnState(keys.previous, accountID, keys.turn, now)
	for _, id := range extraIDs {
		if err := router.affinity.rememberVerified(ctx, accountID, affinityKeys{previous: id}, now); err != nil {
			return err
		}
	}
	return nil
}
