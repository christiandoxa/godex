package main

import (
	"encoding/json"
	"fmt"
)

// A missing previous-response chain must keep every tool-output field intact,
// even across bounded same-owner retries. Never accept an upstream replay
// that silently drops the user's completed tool or rewrites its call ID.
func verifyWebsocketOpaqueChain(b *backend, from int, mode string) error {
	if mode != "ws-unknown-missing" && mode != "ws-bound-missing" {
		return nil
	}
	previous, callID := "resp-tool-missing", "call-missing"
	if mode == "ws-bound-missing" {
		previous, callID = "resp-main", "call-bound"
	}
	for _, attempt := range mustAttempts(b)[from:] {
		if attempt.mode != mode {
			continue
		}
		var value struct {
			Previous string `json:"previous_response_id"`
			Session  string `json:"session_id"`
			Input    []struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
				Output string `json:"output"`
			} `json:"input"`
		}
		if err := json.Unmarshal([]byte(attempt.body), &value); err != nil ||
			value.Previous != previous ||
			value.Session != "sess-conflict" ||
			len(value.Input) != 1 ||
			value.Input[0].Type != "function_call_output" ||
			value.Input[0].CallID != callID ||
			value.Input[0].Output != "done" {
			return fmt.Errorf("%s dropped or rewrote opaque tool/chain context: body=%q err=%v",
				mode, attempt.body, err)
		}
	}
	return nil
}
