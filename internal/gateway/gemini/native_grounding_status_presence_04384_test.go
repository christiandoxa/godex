package gemini

import "testing"

// Tagged 0.438.4 gemini_grounding_source_status picks the first PRESENT
// status member. A present null means no status and suppresses all aliases.
// Earlier Go code instead skipped a null primary and borrowed lower priority
// status, exposing metadata the published kernel deliberately omits.
func TestProdex04384GeminiGroundingStatusUsesFirstPresentValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fields  map[string]any
		want    any
		present bool
	}{
		{"canonical-primary", map[string]any{"urlRetrievalStatus": "SUCCESS", "status": "FAILED"}, "SUCCESS", true},
		{"canonical-secondary", map[string]any{"url_retrieval_status": "SECONDARY", "status": "TERTIARY"}, "SECONDARY", true},
		{"canonical-tertiary", map[string]any{"status": "TERTIARY"}, "TERTIARY", true},
		{"primary-null-blocks-second", map[string]any{"urlRetrievalStatus": nil, "url_retrieval_status": "SECONDARY"}, nil, false},
		{"primary-null-blocks-third", map[string]any{"urlRetrievalStatus": nil, "status": "TERTIARY"}, nil, false},
		{"secondary-null-blocks-third", map[string]any{"url_retrieval_status": nil, "status": "TERTIARY"}, nil, false},
		{"primary-empty-string-present", map[string]any{"urlRetrievalStatus": "", "status": "TERTIARY"}, "", true},
		{"primary-boolean-present", map[string]any{"urlRetrievalStatus": false, "status": "TERTIARY"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := map[string]any{"uri": "https://example.test/source"}
			for key, val := range tc.fields {
				source[key] = val
			}
			sources := make([]map[string]any, 0)
			geminiAppendSource(&sources, map[string]bool{}, source)
			if len(sources) != 1 {
				t.Fatalf("grounding source not emitted: %#v", sources)
			}
			status, present := sources[0]["status"]
			if present != tc.present || (present && status != tc.want) {
				t.Fatalf("grounding status %s emitted=%#v present=%t want=%#v present=%t", tc.name, status, present, tc.want, tc.present)
			}
		})
	}
}

func TestProdex04384GeminiGroundingNullPrimaryStatusCannotBorrowOtherMetadata(t *testing.T) {
	source := map[string]any{"uri": "https://example.test/source", "urlRetrievalStatus": nil, "status": "FAILED"}
	root := map[string]any{"candidates": []any{map[string]any{
		"groundingMetadata": map[string]any{"groundingChunks": []any{map[string]any{"web": source}}},
	}}}
	item := geminiGroundingCall(root, "resp-source")
	if item == nil {
		t.Fatal("grounding source unexpectedly lost")
	}
	action, ok := item["action"].(map[string]any)
	if !ok {
		t.Fatalf("missing grounding action: %#v", item)
	}
	sources, ok := action["sources"].([]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("grounding sources=%#v", action)
	}
	body, ok := sources[0].(map[string]any)
	if !ok {
		t.Fatalf("grounding source shape=%#v", sources[0])
	}
	if _, present := body["status"]; present {
		t.Fatalf("null high-priority status leaked lower-priority status %#v", body)
	}
}
