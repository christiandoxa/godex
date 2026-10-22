package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

type geminiStreamMessageMetadataFixture04384 struct {
	firstRefusal, secondRefusal         string
	firstAnnotations, secondAnnotations string
	wantRefusal, wantAnnotations        string
	only                                bool
}

func geminiStreamMessageMetadataCases04384() []string {
	return []string{"stream-chat-refusal", "stream-chat-annotations", "stream-chat-message-metadata", "stream-chat-message-metadata-only", "stream-chat-message-metadata-null-control", "stream-chat-message-metadata-empty-control", "stream-chat-message-metadata-wrong-control", "stream-chat-message-metadata-absent-control"}
}

func geminiStreamMessageMetadataCase04384(scenario string) bool {
	return slices.Contains(geminiStreamMessageMetadataCases04384(), scenario)
}

func geminiStreamMessageMetadataFixtureFor04384(scenario string) geminiStreamMessageMetadataFixture04384 {
	f := geminiStreamMessageMetadataFixture04384{
		firstRefusal: `"synthetic-"`, secondRefusal: `"refusal<>&/🌍\n"`,
		firstAnnotations: `[{"large":9007199254740993},null]`, secondAnnotations: `[{"unsigned":18446744073709551615},null,[],false,"annotation<>&"]`,
		wantRefusal: `"synthetic-refusal<>&/🌍\n"`, wantAnnotations: `[{"large":9007199254740993},null,{"unsigned":18446744073709551615},null,[],false,"annotation<>&"]`,
	}
	switch scenario {
	case "stream-chat-refusal":
		f.firstAnnotations, f.secondAnnotations, f.wantAnnotations = "", "", ""
	case "stream-chat-annotations":
		f.firstRefusal, f.secondRefusal, f.wantRefusal = "", "", ""
	case "stream-chat-message-metadata-only":
		f.only = true
	case "stream-chat-message-metadata-null-control":
		f.secondRefusal, f.secondAnnotations, f.wantRefusal, f.wantAnnotations = `null`, `null`, f.firstRefusal, f.firstAnnotations
	case "stream-chat-message-metadata-empty-control":
		f.firstRefusal, f.secondRefusal, f.firstAnnotations, f.secondAnnotations, f.wantRefusal, f.wantAnnotations = `""`, `""`, `[]`, `[]`, "", ""
	case "stream-chat-message-metadata-wrong-control":
		f.firstRefusal, f.secondRefusal, f.firstAnnotations, f.secondAnnotations, f.wantRefusal, f.wantAnnotations = `false`, `{}`, `false`, `{}`, "", ""
	case "stream-chat-message-metadata-absent-control":
		f = geminiStreamMessageMetadataFixture04384{}
	}
	return f
}

func geminiStreamMessageMetadataPacket04384(packet, refusal, annotations string, only bool) string {
	var root map[string]any
	_ = json.Unmarshal([]byte(packet), &root)
	delta := root["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	if only {
		delete(delta, "content")
	}
	for key, value := range map[string]string{"refusal": refusal, "annotations": annotations} {
		if value != "" {
			delta[key] = json.RawMessage(value)
		}
	}
	wire, _ := json.Marshal(root)
	return string(wire)
}

func geminiStreamMessageMetadataWire04384(scenario, responseID, itemID string, epoch int64) string {
	wire := geminiChatSSEFixture043711(responseID, itemID, epoch)
	f := geminiStreamMessageMetadataFixtureFor04384(scenario)
	metadata := `"finish_reason":"stop"`
	if f.wantAnnotations != "" {
		metadata = `"annotations":` + f.wantAnnotations + "," + metadata
	}
	if f.wantRefusal != "" {
		metadata += `,"refusal":` + f.wantRefusal
	}
	wire = strings.Replace(wire, `"finish_reason":"stop"`, metadata, 1)
	if !f.only {
		return wire
	}
	created := strings.SplitN(wire, "event: response.output_item.added", 2)[0]
	completed := "event: response.completed" + strings.SplitN(wire, "event: response.completed", 2)[1]
	start, end := strings.Index(completed, `"output":`), strings.Index(completed, `,"usage":`)
	completed = completed[:start] + `"output":[]` + completed[end:]
	return created + strings.Replace(completed, `"sequence_number":5`, `"sequence_number":1`, 1)
}

func geminiStreamMessageMetadataExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	run.Client.Body = geminiStreamMessageMetadataWire04384(scenario, "resp_deepseek_fixture", "msg_gemini_fixture", 111)
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamMessageMetadataInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini message metadata lost exact stdout")
	}
	decoder := sse.NewStrictDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(run.Client.Body)), decoder.Finish()...)
	if decoder.Err() != nil || len(frames) < 2 {
		return errors.New("Gemini message metadata lost SSE framing")
	}
	var created struct {
		CreatedAt int64               `json:"created_at"`
		Response  struct{ ID string } `json:"response"`
	}
	var added struct{ Item struct{ ID string } }
	if json.Unmarshal(frames[0], &created) != nil || created.CreatedAt <= 0 || !strings.HasPrefix(created.Response.ID, "resp_deepseek_") {
		return errors.New("Gemini message metadata lost creation event")
	}
	_ = json.Unmarshal(frames[1], &added)
	if !geminiStreamMessageMetadataFixtureFor04384(scenario).only && !strings.HasPrefix(added.Item.ID, "msg_gemini_") {
		return errors.New("Gemini message metadata lost text item")
	}
	if run.Client.Body != geminiStreamMessageMetadataWire04384(scenario, created.Response.ID, added.Item.ID, created.CreatedAt) {
		return errors.New("Gemini stream changed refusal, annotations or exact lifecycle")
	}
	run.Client.Body = geminiChatSSEFixture043711(created.Response.ID, "msg_gemini_fixture", created.CreatedAt)
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamMessageMetadataNegativeControl04384() bool {
	for _, scenario := range geminiStreamMessageMetadataCases04384() {
		good := geminiStreamMessageMetadataExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"finish_reason":"stop"`, `"finish_reason":"stop","refusal":"forged"`, 1)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"sequence_number":0`, `"sequence_number":9`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamMessageMetadataExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
		if geminiStreamMessageMetadataFixtureFor04384(scenario).wantAnnotations != "" {
			run := geminiStreamMessageMetadataExpected04384(scenario)
			run.Client.Body = strings.Replace(run.Client.Body, `"large":9007199254740993`, `"large":9007199254740992`, 1)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
