package main

import (
	"encoding/json"
	"strings"
)

func deepSeekPreviousResponseCases04384() []string {
	return []string{"deepseek-history-previous-exact", "deepseek-history-previous-request-padded", "deepseek-history-previous-seed-padded", "deepseek-history-previous-request-unpadded", "deepseek-history-simple-previous-padded", "deepseek-history-simple-previous-unknown"}
}

func deepSeekPreviousResponseIDs04384(scenario string) (string, string, bool) {
	seed, previous := "chatcmpl-history-seed", "chatcmpl-history-seed"
	switch scenario {
	case "deepseek-history-previous-exact":
	case "deepseek-history-previous-request-padded":
		previous = " chatcmpl-history-seed "
	case "deepseek-history-previous-seed-padded", "deepseek-history-simple-previous-padded":
		seed, previous = " chatcmpl-history-seed ", " chatcmpl-history-seed "
	case "deepseek-history-previous-request-unpadded":
		seed = " chatcmpl-history-seed "
	case "deepseek-history-simple-previous-unknown":
		previous = "chatcmpl-unknown"
	default:
		return "", "", false
	}
	return seed, previous, true
}

func deepSeekPreviousResponseInput04384(scenario string, phase int) []byte {
	_, previous, _ := deepSeekPreviousResponseIDs04384(scenario)
	text := "current request"
	request := map[string]any{"model": "deepseek-v4-pro", "input": text, "previous_response_id": previous, "stream": false}
	if phase == 2 || !strings.HasPrefix(scenario, "deepseek-history-simple") {
		request["safety_identifier"] = "synthetic_user"
	}
	body, _ := json.Marshal(request)
	return body
}

func deepSeekPreviousResponseRequests04384(scenario string) [][]any {
	user := map[string]any{"role": "user", "content": "history request"}
	current := map[string]any{"role": "user", "content": "current request"}
	requests := [][]any{{user}, {current}}
	if scenario != "deepseek-history-simple-previous-unknown" {
		return requests[:1]
	}
	return requests
}
