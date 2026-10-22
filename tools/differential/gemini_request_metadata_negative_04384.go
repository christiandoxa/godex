package main

import "strings"

func geminiRequestMetadataNegativeControl04384() bool {
	for _, suffix := range geminiRequestMetadataSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		for _, mutate := range []func(string) string{
			func(s string) string {
				return strings.Replace(s, `"metadata":`+fixture.geminiMetadata, `"metadata":{}`, 1)
			},
			func(s string) string { return strings.ReplaceAll(s, `"tag":"synthetic"`, `"tag":"wrong"`) },
			func(s string) string { return strings.ReplaceAll(s, `"decimal":1.0`, `"decimal":1`) },
			func(s string) string { return strings.ReplaceAll(s, `"prompt_cache_key"`, `"wrong_key"`) },
			func(s string) string { return strings.ReplaceAll(s, `<&`, `\u003c\u0026`) },
			func(s string) string { return strings.ReplaceAll(s, `"custom":1`, `"custom":1,"promptTokenCount":2`) },
			func(s string) string {
				if !fixture.geminiChatStream {
					return s
				}
				s = strings.Replace(s, `,"metadata":`+fixture.geminiMetadata, "", 1)
				return strings.Replace(s, `"response":{"id":"resp_deepseek_synthetic"}`, `"response":{"id":"resp_deepseek_synthetic","metadata":`+fixture.geminiMetadata+`}`, 1)
			},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			if len(geminiCompare(scenario, run, run)) != 0 {
				return false
			}
			body := mutate(run.Client.Body)
			if body == run.Client.Body {
				continue
			}
			run.Client.Body, run.Stdout = body, body+"\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
