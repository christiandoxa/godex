package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04356SmartContextDisabledAndUnsupportedPathsStayByteExact(t *testing.T) {
	long := strings.Repeat("duplicate-context-", 400)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	cases := []struct {
		name    string
		enabled bool
		path    string
		header  http.Header
		body    []byte
	}{
		{"disabled", false, "/backend-api/godex/responses", nil, body},
		{"compact", true, "/backend-api/godex/responses/compact", nil, body},
		{"chat", true, "/backend-api/godex/chat/completions", nil, body},
		{"unsupported tokenizer", true, "/backend-api/godex/responses", nil,
			smartContextFixture("gpt-4", []any{messageInput("user", long), messageInput("user", long)})},
		{"too short", true, "/backend-api/godex/responses", nil, []byte(`{"model":"gpt-5.4","input":[]}`)},
		{"invalid json", true, "/backend-api/godex/responses", nil,
			bytes.Repeat([]byte{'{'}, smartContextAdmissionMinBodyBytes)},
		{"wrong content type", true, "/backend-api/godex/responses",
			http.Header{"Content-Type": []string{"text/plain"}}, body},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := prepareSmartContextHTTPBody(
				testCase.enabled, testCase.path, testCase.header, testCase.body,
			)
			if got.Rewritten {
				t.Fatal("request unexpectedly rewritten")
			}
			if !bytes.Equal(got.Body, testCase.body) {
				t.Fatalf("fallback changed bytes: got=%q want=%q", got.Body, testCase.body)
			}
		})
	}
}

func TestProdex04356SmartContextExactHeadersAndContinuationsStayByteExact(t *testing.T) {
	long := strings.Repeat("continuation-context-", 400)
	base := map[string]any{
		"model": "gpt-5.4",
		"input": []any{messageInput("user", long), messageInput("user", long)},
	}
	cases := []struct {
		name   string
		edit   func(map[string]any)
		header http.Header
	}{
		{"godex exact", func(map[string]any) {}, http.Header{"X-Godex-Smart-Context": []string{"ExAcT"}}},
		{"legacy exact", func(map[string]any) {}, http.Header{"X-Prodex-Smart-Context": []string{"exact"}}},
		{"previous response", func(value map[string]any) { value["previous_response_id"] = "resp_123" }, nil},
		{"body session", func(value map[string]any) { value["session_id"] = "session-1" }, nil},
		{"metadata session", func(value map[string]any) {
			value["client_metadata"] = map[string]any{"session_id": "session-2"}
		}, nil},
		{"body turn state", func(value map[string]any) { value["x-codex-turn-state"] = "turn-state" }, nil},
		{"metadata turn state", func(value map[string]any) {
			value["client_metadata"] = map[string]any{"x-codex-turn-state": "turn-state"}
		}, nil},
		{"header turn state", func(map[string]any) {}, http.Header{"X-Codex-Turn-State": []string{"turn-state"}}},
		{"header session", func(map[string]any) {}, http.Header{"Session-Id": []string{"session-3"}}},
		{"turn metadata session", func(map[string]any) {}, http.Header{
			"X-Codex-Turn-Metadata": []string{`{"session_id":"session-4"}`},
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value := deepCloneJSON(base).(map[string]any)
			testCase.edit(value)
			body, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got := prepareSmartContextHTTPBody(
				true, "/backend-api/godex/responses", testCase.header, body,
			)
			if got.Rewritten || !bytes.Equal(got.Body, body) {
				t.Fatalf("exact request changed: rewritten=%t got=%s", got.Rewritten, got.Body)
			}
		})
	}
}

func TestProdex04356SmartContextDedupesAcrossEligibleInputItems(t *testing.T) {
	long := strings.Repeat("alpha beta gamma delta ", 500)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("assistant", "short response"),
		messageInput("user", long),
	})
	got := prepareSmartContextHTTPBody(
		true,
		"/backend-api/godex/responses",
		http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		body,
	)
	if !got.Rewritten {
		t.Fatal("eligible duplicate was not rewritten")
	}
	if bytes.Equal(got.Body, body) {
		t.Fatal("rewrite flag set without changing body")
	}
	var value map[string]any
	if err := json.Unmarshal(got.Body, &value); err != nil {
		t.Fatal(err)
	}
	input := value["input"].([]any)
	if len(input) != 4 {
		t.Fatalf("rewritten input count = %d, want 4", len(input))
	}
	reference := nestedMessageText(input[2])
	if !strings.HasPrefix(reference, "[godex-context-ref v=1 source=original-input[0] digest=sc2:") ||
		!strings.HasSuffix(reference, " bytes="+itoa(len(long))+"]") {
		t.Fatalf("inline reference = %q", reference)
	}
	protocol := input[3].(map[string]any)
	if protocol["role"] != "developer" || protocol["content"] != smartContextInlineReferenceProtocol {
		t.Fatalf("developer protocol = %#v", protocol)
	}
	if bytes.Contains(got.Body, []byte("prodex-context-ref")) ||
		bytes.Contains(got.Body, []byte("Prodex context reference")) {
		t.Fatalf("canonical Godex rewrite leaked Prodex branding: %s", got.Body)
	}

	originalValue, ok := smartContextParseJSON(body)
	if !ok {
		t.Fatal("original fixture did not parse")
	}
	candidateValue, ok := smartContextParseJSON(got.Body)
	if !ok {
		t.Fatal("rewritten fixture did not parse")
	}
	expanded, ok := smartContextExpandInlineReferences(originalValue, candidateValue)
	if !ok || !smartContextRoundTripExact(originalValue, expanded) {
		t.Fatal("rewritten request failed exact round trip")
	}
}

func TestProdex04356SmartContextSystemDeveloperAndSameItemDuplicatesAreNotSources(t *testing.T) {
	long := strings.Repeat("static prelude exact duplicate ", 400)
	sameItem := map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{
			map[string]any{"type": "input_text", "text": long},
			map[string]any{"type": "input_text", "text": long},
		},
	}
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("system", long),
		messageInput("developer", long),
		sameItem,
		messageInput("user", long),
	})
	got := prepareSmartContextHTTPBody(true, "/backend-api/godex/responses", nil, body)
	if !got.Rewritten {
		t.Fatal("later eligible duplicate was not rewritten")
	}
	var value map[string]any
	if err := json.Unmarshal(got.Body, &value); err != nil {
		t.Fatal(err)
	}
	input := value["input"].([]any)
	if nestedMessageText(input[0]) != long || nestedMessageText(input[1]) != long {
		t.Fatal("system/developer duplicate was rewritten")
	}
	same := input[2].(map[string]any)["content"].([]any)
	if same[0].(map[string]any)["text"] != long || same[1].(map[string]any)["text"] != long {
		t.Fatal("duplicate inside the same input item was rewritten")
	}
	reference := nestedMessageText(input[3])
	if !strings.Contains(reference, "source=original-input[2]") {
		t.Fatalf("later eligible duplicate source = %q, want original-input[2]", reference)
	}
}

func TestProdex04356SmartContextSubthresholdSavingsStayByteExact(t *testing.T) {
	// 1024 repeated ASCII bytes are eligible by byte threshold but compress heavily
	// under O200k. The inline protocol overhead keeps net savings below the 128-token floor.
	text := strings.Repeat("a", smartContextDuplicateTextMinBytes)
	body := smartContextFixture("gpt-5.4", []any{
		messageInput("user", text),
		messageInput("user", text),
	})
	got := prepareSmartContextHTTPBody(true, "/backend-api/godex/responses", nil, body)
	if got.Rewritten || !bytes.Equal(got.Body, body) {
		t.Fatalf("subthreshold rewrite = rewritten:%t body:%s", got.Rewritten, got.Body)
	}
}

func TestProdex04356SmartContextDepthAndNodeCapsStayByteExact(t *testing.T) {
	long := strings.Repeat("node-cap-context ", 300)
	deep := any(long)
	for index := 0; index < smartContextJSONMaxDepth+3; index++ {
		deep = []any{deep}
	}
	deepBody := smartContextFixture("gpt-5.4", []any{deep, deep})
	got := prepareSmartContextHTTPBody(true, "/backend-api/godex/responses", nil, deepBody)
	if got.Rewritten || !bytes.Equal(got.Body, deepBody) {
		t.Fatal("over-depth body was rewritten")
	}

	largeInput := make([]any, smartContextJSONMaxNodes+1)
	for index := range largeInput {
		largeInput[index] = 0
	}
	largeInput[0] = messageInput("user", long)
	largeInput[1] = messageInput("user", long)
	nodeBody, err := json.Marshal(map[string]any{"model": "gpt-5.4", "input": largeInput})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodeBody) > smartContextHTTPRewriteMaxBytes {
		t.Fatalf("node-cap fixture too large for intended guard: %d", len(nodeBody))
	}
	got = prepareSmartContextHTTPBody(true, "/backend-api/godex/responses", nil, nodeBody)
	if got.Rewritten || !bytes.Equal(got.Body, nodeBody) {
		t.Fatal("over-node body was rewritten")
	}
}

func TestProdex04356SmartContextDigestRoundTripRejectsTampering(t *testing.T) {
	if got := smartContextArtifactID("abc"); got !=
		"sc2:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sc2 digest = %q", got)
	}
	long := strings.Repeat("tamper target ", 500)
	originalBody := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	got := prepareSmartContextHTTPBody(true, "/backend-api/godex/responses", nil, originalBody)
	if !got.Rewritten {
		t.Fatal("fixture was not rewritten")
	}
	original, _ := smartContextParseJSON(originalBody)
	candidate, _ := smartContextParseJSON(got.Body)
	tampered := deepCloneJSON(candidate).(map[string]any)
	input := tampered["input"].([]any)
	text := nestedMessageText(input[1])
	text = strings.Replace(text, "sc2:", "sc2:0", 1)
	setNestedMessageText(input[1], text)
	if _, ok := smartContextExpandInlineReferences(original, tampered); ok {
		t.Fatal("tampered digest unexpectedly expanded")
	}
}

func TestProdex04356SmartContextO200kModelAndTokenizerParity(t *testing.T) {
	for _, model := range []string{
		"gpt-5.4", "gpt-5", "gpt-4.1", "gpt-4o-2024-11-20",
		"o1-2024-12-17", "o3", "o4-mini", "codex-mini-latest",
		"ft:gpt-5.4:org:model",
	} {
		if !smartContextO200kModel(model) {
			t.Fatalf("O200k model rejected: %s", model)
		}
	}
	for _, model := range []string{"gpt-4", "gpt-3.5-turbo", "text-embedding-3-large", ""} {
		if smartContextO200kModel(model) {
			t.Fatalf("non-O200k model accepted: %s", model)
		}
	}
	if count, ok := smartContextTokenCount([]byte("hello world")); !ok || count != 2 {
		t.Fatalf("o200k token count = %d proven=%t, want 2/true", count, ok)
	}
}

func TestProdex04356SmartContextCriticalSignalGuardFailsClosedOnLoss(t *testing.T) {
	before := []byte("error[E0308]: mismatch src/lib.rs:12:5 @@ -1,2 +1,3 @@ test parses ... FAILED exit code 1 stack backtrace: warning: unused variable")
	after := []byte("plain text")
	if smartContextCriticalSignalsPreserved(before, after) {
		t.Fatal("critical-signal loss was accepted")
	}
	if !smartContextCriticalSignalsPreserved(before, before) {
		t.Fatal("unchanged critical signals were rejected")
	}
}

func smartContextFixture(model string, input []any) []byte {
	value := map[string]any{"model": model, "input": input}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return body
}

func messageInput(role, text string) map[string]any {
	return map[string]any{
		"type": "message",
		"role": role,
		"content": []any{
			map[string]any{"type": "input_text", "text": text},
		},
	}
}

func nestedMessageText(value any) string {
	object := value.(map[string]any)
	content := object["content"].([]any)
	return content[0].(map[string]any)["text"].(string)
}

func setNestedMessageText(value any, text string) {
	object := value.(map[string]any)
	content := object["content"].([]any)
	content[0].(map[string]any)["text"] = text
}

func itoa(value int) string {
	return json.Number(strings.TrimSpace(string(mustJSONNumber(value)))).String()
}

func mustJSONNumber(value int) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
