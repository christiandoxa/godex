package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
)

type geminiUserIDFixture04384 struct{ fields, value, message string }

func geminiUserIDCase04384(scenario string) (geminiUserIDFixture04384, bool) {
	const invalidType = "Gemini OpenAI-compatible user_id must be a string"
	const invalidValue = "Gemini OpenAI-compatible user_id must use only letters, numbers, underscores, or dashes and be at most 512 bytes"
	switch scenario {
	case "request-user-id-absent":
		return geminiUserIDFixture04384{}, true
	case "request-user-id-canonical":
		return geminiUserIDFixture04384{fields: `"user_id":" user_04384 ","user":"ignored_user","safety_identifier":17`, value: "user_04384"}, true
	case "request-user-id-user-alias":
		return geminiUserIDFixture04384{fields: `"user":" user_04384 "`, value: "user_04384"}, true
	case "request-user-id-safety-alias":
		return geminiUserIDFixture04384{fields: `"safety_identifier":" user_04384 "`, value: "user_04384"}, true
	case "request-user-id-blank":
		return geminiUserIDFixture04384{fields: `"user_id":" \t ","user":"ignored_user"`}, true
	case "request-user-id-trim-limit":
		return geminiUserIDFixture04384{fields: `"user_id":" \u2003` + strings.Repeat("u", 512) + `\u00a0 "`, value: strings.Repeat("u", 512)}, true
	case "request-user-id-null":
		return geminiUserIDFixture04384{fields: `"user_id":null,"user":"ignored_user"`, message: invalidType}, true
	case "request-user-id-number", "request-user-id-number-rotation":
		return geminiUserIDFixture04384{fields: `"user_id":7`, message: invalidType}, true
	case "request-user-id-over-limit":
		return geminiUserIDFixture04384{fields: `"user_id":"` + strings.Repeat("u", 513) + `"`, message: invalidValue}, true
	case "request-user-id-invalid-character":
		return geminiUserIDFixture04384{fields: `"user_id":"user.has.dot"`, message: invalidValue}, true
	}
	return geminiUserIDFixture04384{}, false
}

func geminiUserIDInput04384(scenario string) []byte {
	fixture, _ := geminiUserIDCase04384(scenario)
	body := `{"input":"same request","model":"gemini-3.1-pro-preview"`
	if fixture.fields != "" {
		body += "," + fixture.fields
	}
	return []byte(body + "}")
}

func geminiUserIDExpected04384(scenario string) geminiCandidateResult {
	fixture, _ := geminiUserIDCase04384(scenario)
	body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
	result := geminiCandidateResult{Client: exchange{Status: 200, Body: body, Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}}}
	if fixture.message != "" {
		result.Client = exchange{Status: 502, Body: "upstream request failed", Headers: http.Header{"Content-Type": {"text/plain; charset=utf-8"}}}
		result.ExitCode = 2
	} else {
		wire := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false`
		if fixture.value != "" {
			wire += `,"user_id":"` + fixture.value + `"`
		}
		wire += "}"
		var request map[string]any
		_ = json.Unmarshal([]byte(wire), &request)
		result.Upstream = []geminiCapturedRequest{{Method: "POST", Path: "/v1beta/openai/chat/completions", Auth: true, Slot: "single", Body: request, RawBody: wire}}
	}
	result.Stdout = result.Client.Body + "\n"
	return result
}

func geminiUserIDInvariant04384(scenario string, result geminiCandidateResult) error {
	want := geminiUserIDExpected04384(scenario)
	if result.ClientError != "" || len(result.StateIntegrity) != 0 || result.Client.Status != want.Client.Status || result.Client.Body != want.Client.Body ||
		result.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") || result.ExitCode != want.ExitCode || result.Stdout != want.Stdout ||
		!reflect.DeepEqual(result.Upstream, want.Upstream) {
		return errors.New("Gemini user ID lost exact input, alias priority, rejection, credential, response or safe state")
	}
	return nil
}

func geminiUserIDNegativeControl04384() bool {
	for _, scenario := range []string{"request-user-id-user-alias", "request-user-id-null"} {
		run := geminiUserIDExpected04384(scenario)
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		mutations := []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		}
		if len(run.Upstream) != 0 {
			mutations = append(mutations,
				func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
				func(r *geminiCandidateResult) { r.Upstream[0].Slot = "secondary" },
				func(r *geminiCandidateResult) {
					r.Upstream[0].RawBody = strings.Replace(r.Upstream[0].RawBody, "user_id", "user", 1)
				},
				func(r *geminiCandidateResult) { r.Upstream[0].Body = map[string]any{} })
		}
		for _, mutate := range mutations {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			changed.Upstream = append([]geminiCapturedRequest(nil), run.Upstream...)
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
	}
	return true
}
