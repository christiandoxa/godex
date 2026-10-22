package main

import (
	"encoding/json"
	"strings"
)

func geminiFormatNegativeControl04384() bool {
	for _, suffix := range geminiFormatSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		for _, mutation := range [][2]string{
			{"Respond with valid JSON only.", "Return JSON"},
			{`"response_format"`, `"wrong_format"`},
			{geminiFormatDegradedReason04384, "wrong reason"},
			{`"degraded_response_format"`, `"wrong_note"`},
			{`"from":"json_schema"`, `"from":"text"`},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			if len(geminiCompare(scenario, run, run)) != 0 {
				return false
			}
			wire := strings.ReplaceAll(run.Upstream[0].RawBody, mutation[0], mutation[1])
			body := strings.ReplaceAll(run.Client.Body, mutation[0], mutation[1])
			if wire == run.Upstream[0].RawBody && body == run.Client.Body {
				continue
			}
			run.Upstream[0].RawBody = wire
			run.Upstream[0].Body = nil
			_ = json.Unmarshal([]byte(wire), &run.Upstream[0].Body)
			run.Client.Body, run.Stdout = body, body+"\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
