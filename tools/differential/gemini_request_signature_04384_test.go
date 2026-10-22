package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiRequestSignatureOracleRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range geminiRequestSignatureSuffixes04384() {
		scenario := "request-primitive-" + suffix
		want := geminiPrimitiveExpected04384(scenario)
		if len(want.Upstream) == 0 {
			continue
		}
		wire := want.Upstream[0].RawBody
		for _, mutation := range [][2]string{
			{"synthetic-signature", "wrong-signature"},
			{`"extra_content"`, `"ignored"`},
			{`"gemini_thought_signature"`, `"ignored"`},
			{`"synthetic-call"`, `"wrong-call"`},
		} {
			corrupt := strings.Replace(wire, mutation[0], mutation[1], 1)
			if corrupt == wire {
				continue
			}
			changed := geminiPrimitiveExpected04384(scenario)
			changed.Upstream[0].RawBody = corrupt
			if err := json.Unmarshal([]byte(corrupt), &changed.Upstream[0].Body); err != nil {
				t.Fatal(err)
			}
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				t.Fatalf("%s accepted matching request corruption of %s", scenario, mutation[0])
			}
		}
	}
}
