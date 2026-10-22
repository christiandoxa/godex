package chatcompat

import "testing"

func TestMessageRetentionKeepsDefaultRequestContract(t *testing.T) {
	for _, fixture := range []struct {
		name, body, messages string
		strict               bool
	}{
		{"empty", `{"input":[{"type":"message","content":""}]}`, `[{"content":"","role":"user"}]`, false},
		{"system", `{"input":[{"type":"message","role":"system","content":"same request"}]}`, `[{"content":"same request","role":"system"}]`, false},
		{"instructions", `{"instructions":"same request"}`, `[{"content":"same request","role":"system"}]`, false},
		{"empty-array-with-instructions", `{"instructions":"same request","input":[]}`, `[{"content":"same request","role":"system"}]`, false},
		{"ordinary", `{"input":"same request"}`, `[{"content":"same request","role":"user"}]`, true},
		{"null", `{"instructions":"same request","input":null}`, "", false},
		{"missing", `{}`, "", false},
		{"empty-array", `{"input":[]}`, "", false},
		{"unsupported-control", `{"input":"same request","reasoning":{}}`, "", false},
		{"trailing", `{"input":"same request"} {}`, "", false},
	} {
		for _, mode := range []struct {
			name string
			run  func([]byte, string, string) ([]byte, error)
		}{
			{"strict", ResponsesRequest},
			{"preserving", ResponsesRequestPreservingMessages},
		} {
			t.Run(fixture.name+"/"+mode.name, func(t *testing.T) {
				got, err := mode.run([]byte(fixture.body), "synthetic-model", "")
				if fixture.messages == "" || mode.name == "strict" && !fixture.strict {
					if err == nil {
						t.Fatalf("unsupported input accepted: %s", got)
					}
					return
				}
				want := `{"messages":` + fixture.messages + `,"model":"synthetic-model","stream":false}`
				if err != nil || string(got) != want {
					t.Fatalf("message retention differs: got=%s error=%v want=%s", got, err, want)
				}
			})
		}
	}
}
