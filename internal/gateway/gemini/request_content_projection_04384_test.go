package gemini

import "testing"

func TestProdex04384GeminiContentProjectionKeepsSelectedEmptyText(t *testing.T) {
	for _, fixture := range []struct{ content, text string }{
		{`["",{"text":null,"input_text":""},{"output_text":"end"}]`, "\n\nend"},
		{`[7,null,false,["ignored"],{"text":"first","input_text":"later"}]`, "first"},
		{`{"type":"input_text","text":"same request","unused":1.00}`, `{"text":"same request","type":"input_text","unused":1.0}`},
	} {
		request, err := decodeGeminiChatRequest04384([]byte(`{"content":` + fixture.content + `}`))
		if err != nil {
			t.Fatal(err)
		}
		got, err := geminiChatContentText04384(request["content"])
		if err != nil || got != fixture.text {
			t.Fatalf("content projection differs: got=%q error=%v want=%q", got, err, fixture.text)
		}
	}
}

func TestProdex04384GeminiContentPartValidationMatchesTaggedOrder(t *testing.T) {
	for _, fixture := range []struct {
		content string
		valid   bool
	}{
		{`[{"text":"","type":"unknown"}]`, true},
		{`[{"prefix":null,"text":"same request"}]`, true},
		{`[{"type":"input_image"}]`, true},
		{`[7,null,false,[]]`, true},
		{`[{"cache_control":null,"text":"same request"}]`, false},
		{`[{"type":"text","text":7}]`, false},
		{`[{"type":false}]`, false},
		{`[{"ignored":true}]`, false},
	} {
		request, err := decodeGeminiChatRequest04384([]byte(`{"content":` + fixture.content + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := validGeminiChatContent04384(request["content"]); got != fixture.valid {
			t.Fatalf("part validation=%t want=%t content=%s", got, fixture.valid, fixture.content)
		}
	}
}
