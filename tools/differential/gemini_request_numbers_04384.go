package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
)

const geminiRequestNumberInputTokens04384 = `9007199254740991,9007199254740993,18446744073709551615,-9223372036854775808,-9007199254740993,1e+03,-0,0.123456789012345678901234567890,18446744073709551616`
const geminiRequestNumberWireTokens04384 = `9007199254740991,9007199254740993,18446744073709551615,-9223372036854775808,-9007199254740993,1000.0,-0.0,0.12345678901234568,1.8446744073709552e+19`

func geminiRequestNumberCase04384(scenario string) bool {
	return scenario == "request-tool-numbers-buffered" || scenario == "request-tool-numbers-stream"
}

func geminiRequestNumberTool04384(numbers string) string {
	return `[{"function":{"name":"numeric_fixture","parameters":{"properties":{"value":{"enum":[` + numbers + `],"type":"number"}},"type":"object"}},"type":"function"}]`
}

func geminiRequestNumberInput04384(scenario string) []byte {
	stream := "false"
	if scenario == "request-tool-numbers-stream" {
		stream = "true"
	}
	return []byte(`{"input":"same request","model":"gemini-3.1-pro-preview","stream":` + stream + `,"tools":` + geminiRequestNumberTool04384(geminiRequestNumberInputTokens04384) + `}`)
}

func geminiRequestNumberWire04384(scenario string) string {
	stream := "false"
	if scenario == "request-tool-numbers-stream" {
		stream = `true,"stream_options":{"include_usage":true}`
	}
	return `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":` + stream + `,"tools":` + geminiRequestNumberTool04384(geminiRequestNumberWireTokens04384) + `}`
}

func geminiRequestNumberInvariant04384(scenario string, result geminiCandidateResult) error {
	if result.ClientError != "" || len(result.Upstream) != 1 || len(result.StateIntegrity) != 0 || result.Stdout != result.Client.Body+"\n" {
		return errors.New("Gemini numeric request lost one original attempt, stdout or safe state")
	}
	request := result.Upstream[0]
	wire := geminiRequestNumberWire04384(scenario)
	var want map[string]any
	if json.Unmarshal([]byte(wire), &want) != nil || request.Method != "POST" || request.Path != "/v1beta/openai/chat/completions" ||
		!request.Auth || request.Slot != "single" || request.RawBody != wire || !reflect.DeepEqual(request.Body, want) {
		return errors.New("Gemini numeric request bytes, credential or original payload differ")
	}
	body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
	if result.ExitCode != 0 || result.Client.Status != 200 || result.Client.Body != body || result.Client.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
		return errors.New("Gemini numeric request response body, type, exit or status differ")
	}
	return nil
}

func geminiRequestNumberNegativeControl04384() bool {
	for _, scenario := range []string{"request-tool-numbers-buffered", "request-tool-numbers-stream"} {
		body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
		wire := geminiRequestNumberWire04384(scenario)
		var request map[string]any
		if json.Unmarshal([]byte(wire), &request) != nil {
			return false
		}
		run := geminiCandidateResult{Client: exchange{Status: 200, Body: body, Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}},
			Stdout: body + "\n", Upstream: []geminiCapturedRequest{{Method: "POST", Path: "/v1beta/openai/chat/completions", Auth: true, Slot: "single", Body: request, RawBody: wire}}}
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) {
				r.Upstream[0].RawBody = strings.Replace(wire, "9007199254740993", "9007199254740992", 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = strings.Replace(wire, "1000.0", "1000", 1) },
			func(r *geminiCandidateResult) { r.Upstream[0].Body = map[string]any{} },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream[0].Slot = "secondary" },
			func(r *geminiCandidateResult) { r.Upstream[0].Path = "/wrong" },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.ClientError = "missing exchange" },
		} {
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
