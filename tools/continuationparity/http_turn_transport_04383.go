package main

import (
	"errors"
	"fmt"
	"path/filepath"
)

const httpTurnUnavailable04383 = `{"error":{"code":"service_unavailable","message":"Runtime proxy could not secure a healthy upstream profile before the pre-commit retry budget was exhausted. Retry the request."}}`

func runHTTPResponsesTurnProduct04383(root string, p product) error {
	for _, scenario := range []string{"turn-only", "with-session", "previous-response", "unknown-previous"} {
		if err := runHTTPTransportCase04383(filepath.Join(root, scenario), p, scenario, "/responses"); err != nil {
			return fmt.Errorf("%s: %w", scenario, err)
		}
	}
	return runHTTPRecoverySessionProduct04383(filepath.Join(root, "session-retry"), p)
}

func verifyHTTPResponsesTurn04383(scenario string, result httpTransportResult04383) error {
	status, body, turn, unrelated := 503, httpTurnUnavailable04383, "main", "main"
	if scenario == "previous-response" {
		status, body, turn, unrelated = 400, compactRetry04383, "", ""
	} else if scenario != "turn-only" && scenario != "with-session" && scenario != "unknown-previous" {
		return errors.New("unknown Responses turn transport scenario")
	}
	if result.invalid || len(result.attempts) != 1 || result.reply.status != status || result.reply.body != body {
		return fmt.Errorf("HTTP Responses turn auth, dispatch count or terminal changed: invalid=%t attempts=%d status=%d body=%s", result.invalid, len(result.attempts), result.reply.status, result.reply.body)
	}
	attempt := result.attempts[0]
	if !attempt.ready || attempt.owner != "account-a" || attempt.turn != "turn-a" || attempt.body != httpTransportBody04383(scenario) {
		return errors.New("HTTP Responses turn changed authenticated readiness, owner or opaque wire")
	}
	if result.owners["unknown"] != "" || result.owners["turn"] != turn || result.owners["response"] != "" || result.owners["unrelated"] != unrelated || result.owners["session"] != "main" {
		return fmt.Errorf("HTTP Responses turn durable ownership changed: %v", result.owners)
	}
	return nil
}
