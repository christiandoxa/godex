package main

import (
	"encoding/json"
	"strings"
)

type rawKeyIdentityFixture04384 struct {
	previous, session string
	reject, compact   bool
}

const rawKeyIdentityResponse04384 = `{"created_at":1,"id":"chatcmpl-differential","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","object":"response","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`

func rawKeyIdentityCase04384(name string) (rawKeyIdentityFixture04384, bool) {
	switch name {
	case "deepseek-raw-key-id-limit":
		return rawKeyIdentityFixture04384{previous: strings.Repeat("x", 1024)}, true
	case "deepseek-raw-key-id-over-limit":
		return rawKeyIdentityFixture04384{previous: strings.Repeat("x", 1025), reject: true}, true
	case "deepseek-raw-key-id-unicode-limit":
		return rawKeyIdentityFixture04384{previous: strings.Repeat("雪", 341) + "x"}, true
	case "deepseek-raw-key-id-unicode-over":
		return rawKeyIdentityFixture04384{previous: strings.Repeat("雪", 342), reject: true}, true
	case "deepseek-raw-key-id-control":
		return rawKeyIdentityFixture04384{previous: "synthetic\x00context", reject: true}, true
	case "deepseek-raw-key-id-mixed":
		return rawKeyIdentityFixture04384{previous: "chatcmpl-unknown", session: strings.Repeat("x", 1025), reject: true}, true
	case "deepseek-raw-key-id-blank":
		return rawKeyIdentityFixture04384{previous: " \t "}, true
	case "deepseek-raw-key-id-local-compact":
		return rawKeyIdentityFixture04384{previous: strings.Repeat("x", 1025), compact: true}, true
	}
	return rawKeyIdentityFixture04384{}, false
}

func rawKeyIdentityRequest04384(name string, body []byte) ([]byte, bool) {
	fixture, known := rawKeyIdentityCase04384(name)
	if !known {
		return nil, false
	}
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return nil, false
	}
	request["previous_response_id"] = fixture.previous
	if fixture.session != "" {
		request["session_id"] = fixture.session
	}
	encoded, err := json.Marshal(request)
	return encoded, err == nil
}

func rawKeyIdentityInvariants04384(scenario scenarioResult) []string {
	fixture, known := rawKeyIdentityCase04384(scenario.Name)
	if !known || len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".case_or_run_count"}
	}
	var failures []string
	for _, run := range scenario.Runs {
		prefix := scenario.Name + "." + run.Name
		valid := run.ExitStatus == 0 && run.Client.Status == 200 && run.Client.Body == rawKeyIdentityResponse04384
		contentType := "application/json; charset=utf-8"
		attempts := 1
		if fixture.reject {
			valid, attempts = rawKeyProbeRejected(run), 0
			contentType = "application/json"
		} else if fixture.compact {
			valid, attempts = validRawKeyLocalRoute04384("resume-compact", run), 0
		}
		if !valid || run.Client.Headers.Get("Content-Type") != contentType || strings.TrimSuffix(run.Stdout, "\n") != run.Client.Body {
			failures = append(failures, prefix+".response")
		}
		if len(run.Upstream) != attempts || run.Retries != 0 || len(run.StateIntegrity) != 0 {
			failures = append(failures, prefix+".attempts_or_state")
		}
		for _, request := range run.Upstream {
			if request.Method != "POST" || request.Path != "/v1/chat/completions" || !request.AuthOK ||
				request.KeySlot != "primary" || !validFixtureRequest(request.Body) {
				failures = append(failures, prefix+".request_or_credential")
			}
		}
	}
	return failures
}
