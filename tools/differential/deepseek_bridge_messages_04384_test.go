package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekBridgeMessagesRejectMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekBridgeMessageSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			if !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) {
				t.Fatal("bridge message fixture is absent from the default registry")
			}
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("bridge message fixture lost valid original input or golden wire")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"content":"preface"`, `"content":"forged"`, 1)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching bridge message corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("bridge message fixtures weakened primitive wire/state assertions")
	}
}

func TestDeepSeekBridgeMessagePendingRejectsMatchingCorruption04384(t *testing.T) {
	const scenario = "deepseek-history-message-pending"
	if !deepSeekHistoryCase04384(scenario) || !strings.Contains(string(deepSeekHistoryInput04384(scenario, 0)), "call-inline") {
		t.Fatal("pending message fixture lost its initial embedded call")
	}
	good := deepSeekHistoryExpected04384(scenario)
	if len(good.Upstream) != 3 || len(deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}})) != 0 {
		t.Fatal("pending message golden lost its three original requests")
	}
	for _, index := range []int{1, 2} {
		bad := deepSeekHistoryExpected04384(scenario)
		bad.Upstream[index].Body = strings.Replace(bad.Upstream[index].Body, `"content":"preface"`, `"content":"preface","tool_calls":[]`, 1)
		if len(deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
			t.Fatal("accepted matching leaked embedded calls in pending history")
		}
	}
	if !deepSeekHistoryNegativeControl04384() {
		t.Fatal("pending message fixture weakened history invariants")
	}
}
