package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeepSeekHistoryProjectionRejectsMatchingCorruption(t *testing.T) {
	if !deepSeekHistoryNegativeControl04384() {
		t.Fatal("history seed/request/state corruption escaped independent checks")
	}
}

func TestDeepSeekHistoryCallIDsRejectMatchingCorruption04384(t *testing.T) {
	for _, scenario := range []string{"deepseek-history-id-padded", "deepseek-history-id-output-padded", "deepseek-history-id-call-padded", "deepseek-history-simple-padded-pending"} {
		var input map[string]any
		if err := json.Unmarshal(deepSeekHistoryInput04384(scenario, 1), &input); err != nil {
			t.Fatal(err)
		}
		_, outputID := deepSeekHistoryCallIDs04384(scenario)
		if input["input"].([]any)[0].(map[string]any)["call_id"] != outputID {
			t.Fatal("history fixture changed the opaque output ID")
		}
		good := deepSeekHistoryExpected04384(scenario)
		if failures := deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
			t.Fatal(failures)
		}
		bad := deepSeekHistoryExpected04384(scenario)
		bad.Client.Seed.Body = strings.ReplaceAll(bad.Client.Seed.Body, "call-history", "forged")
		if len(deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
			t.Fatal("accepted matching cached call ID corruption")
		}
		bad = deepSeekHistoryExpected04384(scenario)
		bad.Upstream[len(bad.Upstream)-1].Body = strings.Replace(bad.Upstream[len(bad.Upstream)-1].Body, `"messages":[`, `"messages":[{"role":"tool","tool_call_id":"forged","content":"unexpected"},`, 1)
		if len(deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
			t.Fatal("accepted matching forged history owner")
		}
	}
}
