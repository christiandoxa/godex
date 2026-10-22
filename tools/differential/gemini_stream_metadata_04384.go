package main

import (
	"errors"
	"slices"
	"strings"
)

type geminiStreamMetadataFixture04384 struct {
	firstFingerprint, secondFingerprint string
	firstLogprobs, secondLogprobs       string
	metadata                            string
}

func geminiStreamMetadataCases04384() []string {
	return []string{"stream-chat-metadata", "stream-chat-metadata-update", "stream-chat-metadata-null", "stream-chat-metadata-control", "stream-chat-metadata-false"}
}

func geminiStreamMetadataCase04384(scenario string) bool {
	return slices.Contains(geminiStreamMetadataCases04384(), scenario)
}

func geminiStreamMetadataFixtureFor04384(scenario string) geminiStreamMetadataFixture04384 {
	const logprobs = `{"token_id":9007199254740993,"value":-0.0}`
	fixture := geminiStreamMetadataFixture04384{
		firstFingerprint: `" fp <>& "`, firstLogprobs: logprobs,
		metadata: `,"logprobs":` + logprobs + `,"system_fingerprint":" fp <>& "`,
	}
	switch scenario {
	case "stream-chat-metadata-update":
		fixture.firstFingerprint, fixture.secondFingerprint = `"old"`, fixture.firstFingerprint
		fixture.firstLogprobs, fixture.secondLogprobs = `{"old":true}`, fixture.firstLogprobs
	case "stream-chat-metadata-null":
		fixture.secondFingerprint, fixture.secondLogprobs = `""`, `null`
	case "stream-chat-metadata-control":
		fixture.firstFingerprint, fixture.firstLogprobs, fixture.metadata = `12`, `null`, ""
	case "stream-chat-metadata-false":
		fixture.firstFingerprint, fixture.firstLogprobs, fixture.metadata = `null`, `false`, `,"logprobs":false`
	}
	return fixture
}

func geminiStreamMetadataPacket04384(packet, fingerprint, logprobs string) string {
	if fingerprint != "" {
		packet = strings.Replace(packet, `"id":`, `"system_fingerprint":`+fingerprint+`,"id":`, 1)
	}
	if logprobs != "" {
		packet = strings.Replace(packet, `"delta":`, `"logprobs":`+logprobs+`,"delta":`, 1)
	}
	return packet
}

func geminiStreamMetadataExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	fixture := geminiStreamMetadataFixtureFor04384(scenario)
	run.Client.Body = strings.Replace(run.Client.Body, `"finish_reason":"stop"`, `"finish_reason":"stop"`+fixture.metadata, 1)
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamMetadataInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini stream metadata lost exact stdout")
	}
	fixture := geminiStreamMetadataFixtureFor04384(scenario)
	if fixture.metadata != "" {
		metadata := `"finish_reason":"stop"` + fixture.metadata
		if strings.Count(run.Client.Body, metadata) != 1 {
			return errors.New("Gemini stream lost exact retained choice or chunk metadata")
		}
		run.Client.Body = strings.Replace(run.Client.Body, metadata, `"finish_reason":"stop"`, 1)
		run.Stdout = run.Client.Body + "\n"
	}
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamMetadataNegativeControl04384() bool {
	for _, scenario := range geminiStreamMetadataCases04384() {
		good := geminiStreamMetadataExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "finish_reason", "wrong")
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamMetadataExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
		if strings.Contains(good.Client.Body, "9007199254740993") {
			good.Client.Body = strings.ReplaceAll(good.Client.Body, "9007199254740993", "9007199254740992")
			good.Stdout = good.Client.Body + "\n"
			if len(geminiCompare(scenario, good, good)) == 0 {
				return false
			}
		}
	}
	return true
}
