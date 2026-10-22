package deepseek

import (
	"strings"
	"testing"
)

func TestProdex04384DeepSeekStopAdmissionAndAliases(t *testing.T) {
	const stops16 = `["s01","s02","s03","s04","s05","s06","s07","s08","s09","s10","s11","s12","s13","s14","s15","s16"]`
	for _, fixture := range []struct{ name, fields, want string }{
		{"string", `"stop":"STOP"`, `"STOP"`},
		{"empty_string", `"stop":""`, `""`},
		{"empty_array", `"stop":[]`, `[]`},
		{"snake", `"stop_sequences":[" STOP ",""]`, `[" STOP ",""]`},
		{"camel", `"stopSequences":"CAMEL"`, `"CAMEL"`},
		{"precedence", `"stop":"PRIMARY","stop_sequences":null,"stopSequences":false`, `"PRIMARY"`},
		{"alias_precedence", `"stop_sequences":"SECOND","stopSequences":"THIRD"`, `"SECOND"`},
		{"shadow_null", `"stop":null,"stop_sequences":"SECOND"`, ""},
		{"null", `"stop":null`, ""},
		{"boolean", `"stop":false`, ""},
		{"array_number", `"stop":["STOP",1]`, ""},
		{"limit", `"stop":` + stops16, stops16},
		{"over_limit", `"stop":` + strings.TrimSuffix(stops16, "]") + `,"s17"]`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"same request",`+fixture.fields+`}`), RequestOptions{})
			if fixture.want == "" {
				if err == nil {
					t.Fatal("invalid stop was accepted")
				}
				return
			}
			want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stop":` + fixture.want + `,"stream":false}`
			if err != nil || string(body) != want {
				t.Fatalf("stop request = %s, error=%v, want %s", body, err, want)
			}
		})
	}
}
