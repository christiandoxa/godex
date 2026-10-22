package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekRuntimeMessageContent(t *testing.T) {
	for _, fixture := range []struct{ name, prefix, value, want string }{
		{"object", "", `{"type":"input_text","text":"alpha"}`, `"{\"text\":\"alpha\",\"type\":\"input_text\"}"`},
		{"empty object text", "", `{"text":""}`, `"{\"text\":\"\"}"`},
		{"simple empty part", "", `[{"text":""},{"text":"last"}]`, `"\nlast"`},
		{"normal empty part", `{"type":"message","role":"developer","content":"synthetic instruction"},`, `[{"text":""},{"text":"last"}]`, `"\nlast"`},
		{"string and object empties", "", `["",{"text":""},"last"]`, `"\n\nlast"`},
		{"text control", "", `[{"text":"alpha"}]`, `"alpha"`},
		{"standalone control", "", `{"type":"input_text","text":"alpha"}`, `"alpha"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := `{"input":[` + fixture.prefix + `{"type":"message","role":"user","content":` + fixture.value + `},{"role":"user","content":"same request"}]}`
			contains := func(string) bool { return false }
			if fixture.name == "standalone control" {
				contains = nil
			}
			translated, err := translateResponsesRequestWithHistory([]byte(input), RequestOptions{}, nil, contains)
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Messages []struct{ Content json.RawMessage }
			}
			if err := json.Unmarshal(translated.Body, &request); err != nil {
				t.Fatal(err)
			}
			index := 0
			if fixture.prefix != "" {
				index = 1
			}
			if len(request.Messages) != index+2 || string(request.Messages[index].Content) != fixture.want {
				t.Fatal("runtime message content lost JSON text, empty parts or standalone compatibility")
			}
		})
	}
}
