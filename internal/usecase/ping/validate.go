package ping

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

type protocolState struct {
	threadStarted  bool
	turnStarted    bool
	agentMessage   bool
	turnCompleted  bool
	toolActivity   bool
	turnFailure    string
	effectiveModel string
}

func validateJSONL(content []byte) (pingmodel.Status, string, string) {
	state := protocolState{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		consumePingEvent(scanner.Bytes(), &state)
	}
	if state.turnFailure != "" {
		status := classifyFailure(state.turnFailure)
		if status == pingmodel.ProcessFailed {
			status = pingmodel.TurnFailed
		}
		return status, statusDetail(status), state.effectiveModel
	}
	if state.toolActivity || !state.threadStarted || !state.turnStarted || !state.turnCompleted {
		return pingmodel.ProtocolFailed, statusDetail(pingmodel.ProtocolFailed), state.effectiveModel
	}
	if !state.agentMessage {
		return pingmodel.UnexpectedResponse, statusDetail(pingmodel.UnexpectedResponse), state.effectiveModel
	}
	return pingmodel.Pass, statusDetail(pingmodel.Pass), state.effectiveModel
}

func consumePingEvent(line []byte, state *protocolState) {
	var event struct {
		Type  string `json:"type"`
		Model string `json:"model"`
		Item  struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Model string `json:"model"`
		} `json:"item"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &event) != nil {
		return
	}
	if event.Model != "" {
		state.effectiveModel = event.Model
	} else if event.Item.Model != "" {
		state.effectiveModel = event.Item.Model
	}
	switch event.Type {
	case "thread.started":
		state.threadStarted = true
	case "turn.started":
		state.turnStarted = true
	case "turn.completed":
		state.turnCompleted = true
	case "turn.failed":
		state.turnFailure = strings.TrimSpace(event.Error.Message)
	case "item.completed":
		consumeCompletedItem(event.Item.Type, event.Item.Text, state)
	}
}

func consumeCompletedItem(itemType, text string, state *protocolState) {
	switch itemType {
	case "agent_message":
		if strings.TrimSpace(text) != "" {
			state.agentMessage = true
		}
	case "reasoning":
		// Reasoning is model output but not a user-visible response or tool action.
	case "":
	default:
		state.toolActivity = true
	}
}

func classifyFailure(text string) pingmodel.Status {
	value := strings.ToLower(text)
	switch {
	case strings.Contains(value, "http 503"), strings.Contains(value, "http 502"), strings.Contains(value, "http 504"), strings.Contains(value, "upstream unavailable"), strings.Contains(value, "overloaded"):
		return pingmodel.UpstreamOverloaded
	case strings.Contains(value, "usage_limit_reached"), strings.Contains(value, "insufficient_quota"):
		return pingmodel.QuotaExhausted
	case strings.Contains(value, "http 429"), strings.Contains(value, "rate limit"), strings.Contains(value, "rate_limit"):
		return pingmodel.RateLimited
	case strings.Contains(value, "http 401"), strings.Contains(value, "http 403"), strings.Contains(value, "unauthorized"), strings.Contains(value, "authentication failed"):
		return pingmodel.AuthFailed
	case strings.Contains(value, "no such host"), strings.Contains(value, "dns"), strings.Contains(value, "name resolution"), strings.Contains(value, "lookup "):
		return pingmodel.DNSFailed
	case strings.Contains(value, "tls"), strings.Contains(value, "x509"), strings.Contains(value, "certificate"):
		return pingmodel.TLSFailed
	case strings.Contains(value, "timed out"), strings.Contains(value, "timeout"):
		return pingmodel.Timeout
	case strings.Contains(value, "model") && (strings.Contains(value, "not found") || strings.Contains(value, "unavailable")):
		return pingmodel.ModelUnavailable
	case strings.Contains(value, "failed to start"):
		return pingmodel.SpawnFailed
	default:
		return pingmodel.ProcessFailed
	}
}
