package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiCompactStreamUsesActualPublicSeedID(t *testing.T) {
	seed := geminiCompactStreamSeedExchange04384("compact-owner-stream")
	body, err := geminiCompactStreamInput04384("compact-owner-stream", seed)
	var request map[string]any
	if err != nil || json.Unmarshal(body, &request) != nil || request["previous_response_id"] != "resp_deepseek_fixture" {
		t.Fatalf("stream seed ID lost: %s, %v", body, err)
	}
	for _, invalid := range []string{"", "data: [DONE]\n\n", `data: {"type":"response.created","response":{"id":"wrong"}}` + "\n\n"} {
		if _, err := geminiCompactStreamInput04384("compact-owner-stream", exchange{Body: invalid}); err == nil {
			t.Fatal("missing public stream ID was accepted")
		}
	}
}

func TestGeminiCompactStreamRejectsMatchingSeedCorruption(t *testing.T) {
	for _, scenario := range []string{"compact-owner-stream", "compact-owner-stream-eof", "compact-owner-stream-unbound"} {
		run := geminiCompactExpected04384(scenario)
		if len(geminiCompare(scenario, run, run)) != 0 {
			t.Fatalf("valid stream seed rejected: %s", scenario)
		}
		run.Client.Seed.Body = strings.Replace(run.Client.Seed.Body, "synthetic-", "forged-", 1)
		if len(geminiCompare(scenario, run, run)) == 0 {
			t.Fatalf("matching stream corruption accepted: %s", scenario)
		}
	}
}
